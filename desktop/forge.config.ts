import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import type { ForgeConfig } from "@electron-forge/shared-types";
import { MakerDeb } from "@electron-forge/maker-deb";
import { MakerDMG } from "@electron-forge/maker-dmg";
import { MakerRpm } from "@electron-forge/maker-rpm";
import { MakerZIP } from "@electron-forge/maker-zip";
import { AutoUnpackNativesPlugin } from "@electron-forge/plugin-auto-unpack-natives";
import { VitePlugin } from "@electron-forge/plugin-vite";

const repositoryRoot = resolve(__dirname, "..");
const binDir = resolve(repositoryRoot, "build/bin");
const windows = process.platform === "win32";
const executableSuffix = windows ? ".exe" : "";

/**
 * Files that ship beside the application. The Go backend locates sing-box and
 * the privileged helper next to its own executable or under a sibling
 * `resources` directory, both of which Electron's resources path satisfies.
 */
function extraResources(): string[] {
  const required = [
    resolve(binDir, `kubeloop-desktop-host${executableSuffix}`),
    resolve(binDir, `sing-box${executableSuffix}`),
  ];
  for (const path of required) {
    if (!existsSync(path)) {
      throw new Error(
        `Required packaging asset missing: ${path}. ` +
          `Run "npm run build:host" and "go run ./build/stage-package-assets.go" first.`,
      );
    }
  }

  const optional = [
    resolve(binDir, "LICENSE.sing-box.txt"),
    resolve(repositoryRoot, "packaging/icons/appicon.png"),
    resolve(repositoryRoot, "build/embedded", `kubeloop-helper${executableSuffix}`),
    // sing-box needs these sidecars on Windows only.
    ...(windows
      ? [resolve(binDir, "libcronet.dll"), resolve(binDir, "wintun.dll")]
      : []),
  ];
  return [...required, ...optional.filter(existsSync)];
}

/** The directory Electron Packager wrote for the current platform. */
function packagedPath(): string {
  return resolve(__dirname, `out/KubeLoop-${process.platform}-${process.arch}`);
}

/**
 * Archive the packaged application as a tarball.
 *
 * The zip maker is not usable here: it drops the executable bits, and on Linux
 * the setuid bit on `chrome-sandbox` that Electron needs to launch. A tarball
 * also keeps the asset names the install scripts and Homebrew cask already
 * expect.
 */
function makeTarball(version: string): string {
  const outputDir = resolve(__dirname, "out/make/tar");
  mkdirSync(outputDir, { recursive: true });
  const output = resolve(outputDir, `KubeLoop-${process.platform}-${process.arch}-${version}.tar.gz`);
  // macOS publishes the bundle; Linux publishes the directory contents, so an
  // extraction lands the executable straight in the target directory.
  const member = process.platform === "darwin" ? "KubeLoop.app" : ".";
  execFileSync("tar", ["-czf", output, "-C", packagedPath(), member], { stdio: "inherit" });
  return output;
}

/**
 * Re-sign the macOS bundle after packaging.
 *
 * Copying the extra resources in breaks the seal Electron Packager left, and
 * Gatekeeper then refuses to open the app ("damaged" / will not launch). An
 * ad-hoc deep signature restores it; release signing with a real identity is
 * tracked separately in ADR 0023.
 */
function signMacOSBundle(): void {
  const bundle = resolve(packagedPath(), "KubeLoop.app");
  execFileSync("codesign", ["--force", "--deep", "--sign", "-", bundle], { stdio: "inherit" });
  execFileSync("codesign", ["--verify", "--deep", "--strict", bundle], { stdio: "inherit" });
}

/**
 * Build the Windows installer.
 *
 * Electron Forge has no NSIS maker, and the project's installer does work no
 * stock maker covers: it installs the privileged helper and runs
 * `kubeloop-helper.exe uninstall` to stop that service when the app is removed.
 * So the existing NSIS script is driven directly from a hook instead.
 */
function makeWindowsInstaller(version: string): string {
  const installerDir = resolve(repositoryRoot, "packaging/windows");
  const outputDir = resolve(__dirname, "out/make/nsis");
  mkdirSync(outputDir, { recursive: true });
  const architecture = process.arch === "arm64" ? "arm64" : "amd64";
  const output = resolve(outputDir, `KubeLoop-${architecture}-installer.exe`);

  execFileSync(
    "makensis",
    [
      `-DVERSION=${version}`,
      `-DARCH=${architecture}`,
      `-DSOURCE_DIR=${packagedPath()}`,
      `-DOUT_FILE=${output}`,
      resolve(installerDir, "kubeloop.nsi"),
    ],
    { cwd: installerDir, stdio: "inherit" },
  );
  return output;
}

const config: ForgeConfig = {
  packagerConfig: {
    name: "KubeLoop",
    executableName: windows ? "KubeLoop" : "kubeloop",
    // Packager wants the platform's own icon format; Linux takes its icon
    // from the maker options instead.
    icon: resolve(repositoryRoot, windows ? "packaging/icons/appicon.ico" : "packaging/icons/appicon.icns"),
    appBundleId: "dev.fengqi.kube-loop",
    appCategoryType: "public.app-category.developer-tools",
    protocols: [{ name: "KubeLoop", schemes: ["kubeloop"] }],
    extraResource: extraResources(),
    asar: true,
  },
  rebuildConfig: {},
  makers: [
    // Only Windows ships a zip; the other platforms use tarballs, built in the
    // postMake hook below so file modes survive.
    new MakerZIP({}, ["win32"]),
    new MakerDMG({ name: "KubeLoop", icon: resolve(repositoryRoot, "packaging/icons/appicon.icns") }, ["darwin"]),
    new MakerDeb({
      options: {
        name: "kubeloop-desktop",
        productName: "KubeLoop",
        genericName: "Kubernetes Development Network",
        categories: ["Development"],
        mimeType: ["x-scheme-handler/kubeloop"],
        icon: resolve(repositoryRoot, "packaging/icons/appicon.png"),
      },
    }),
    new MakerRpm({
      options: {
        name: "kubeloop-desktop",
        productName: "KubeLoop",
        categories: ["Development"],
        mimeType: ["x-scheme-handler/kubeloop"],
        icon: resolve(repositoryRoot, "packaging/icons/appicon.png"),
      },
    }),
  ],
  hooks: {
    async postPackage() {
      if (process.platform === "darwin") {
        signMacOSBundle();
      }
    },
    async postMake(_forgeConfig, results) {
      const version = String(process.env.npm_package_version ?? "0.0.0");
      const extra = windows ? [makeWindowsInstaller(version)] : [makeTarball(version)];
      return [
        ...results,
        {
          artifacts: extra,
          packageJSON: { version },
          platform: process.platform,
          arch: process.arch,
          status: "ready" as const,
        },
      ];
    },
  },
  plugins: [
    new AutoUnpackNativesPlugin({}),
    new VitePlugin({
      build: [
        { entry: "src/main/main.ts", config: "vite.main.config.ts", target: "main" },
        { entry: "src/main/preload.ts", config: "vite.preload.config.ts", target: "preload" },
      ],
      renderer: [{ name: "main_window", config: "vite.renderer.config.ts" }],
    }),
  ],
};

export default config;
