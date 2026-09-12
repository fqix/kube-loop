// Development launcher for the Electron desktop shell.
//
// 1. Builds the Management Plane assets and the local dev stack images.
// 2. Builds the Go backend, helper and sing-box into build/bin.
// 3. Hands over to `electron-vite dev`, which bundles main/preload/renderer
//    with HMR and launches Electron.
//
// KUBELOOP_SKIP_DEV_STACK=1 skips step 1 for quick UI iteration.
import { spawn, spawnSync } from "node:child_process"
import { existsSync, readFileSync, writeFileSync } from "node:fs"
import { createRequire } from "node:module"
import path from "node:path"
import process from "node:process"
import { fileURLToPath } from "node:url"

const rootDirectory = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const frontendDirectory = path.join(rootDirectory, "frontend")
const require = createRequire(path.join(rootDirectory, "package.json"))
const electronBinary = require("electron")
const electronVite = path.join(rootDirectory, "node_modules", ".bin", process.platform === "win32" ? "electron-vite.cmd" : "electron-vite")

function run(command, args, cwd = rootDirectory) {
  const result = spawnSync(command, args, { cwd, stdio: "inherit" })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${command} ${args.join(" ")} exited with status ${result.status}`)
}

// Electron's development bundle does not declare the kubeloop:// scheme, so
// macOS would never deliver the OAuth callback via open-url. Patch the
// bundle's Info.plist once; setAsDefaultProtocolClient handles registration.
function ensureDevelopmentProtocol() {
  if (process.platform !== "darwin") return
  const plist = path.resolve(path.dirname(electronBinary), "..", "Info.plist")
  if (!existsSync(plist)) return
  const contents = readFileSync(plist, "utf8")
  if (contents.includes("<string>kubeloop</string>")) return
  const urlTypes = `\t<key>CFBundleURLTypes</key>
\t<array>
\t\t<dict>
\t\t\t<key>CFBundleURLName</key>
\t\t\t<string>dev.fengqi.kube-loop</string>
\t\t\t<key>CFBundleURLSchemes</key>
\t\t\t<array>
\t\t\t\t<string>kubeloop</string>
\t\t\t</array>
\t\t</dict>
\t</array>
`
  writeFileSync(plist, contents.replace(/<\/dict>\n<\/plist>\s*$/, `${urlTypes}</dict>\n</plist>\n`))
  console.log(`==> Declared kubeloop:// in ${plist}`)
}

let child
let shuttingDown = false

function shutdown(signal, exitCode = 0) {
  if (shuttingDown) return
  shuttingDown = true
  if (child && child.exitCode === null && !child.killed) child.kill(signal)
  process.exit(exitCode)
}

try {
  if (!process.env.KUBELOOP_SKIP_DEV_STACK) {
    run("npm", ["--prefix", frontendDirectory, "run", "build:admin"])
    run("go", ["run", "./build/gateway-dev.go"])
  }
  run("go", ["run", "./build/desktop-backend.go"])
  ensureDevelopmentProtocol()

  child = spawn(electronVite, ["dev", ...process.argv.slice(2)], {
    cwd: rootDirectory,
    stdio: "inherit",
    shell: process.platform === "win32",
  })
  child.on("error", (error) => {
    console.error(`Failed to start electron-vite: ${error.message}`)
    shutdown("SIGTERM", 1)
  })
  child.on("exit", (code, signal) => shutdown(signal || "SIGTERM", signal ? 0 : (code ?? 0)))
} catch (error) {
  console.error(`Failed to prepare Electron development mode: ${error.message}`)
  shutdown("SIGTERM", 1)
}

process.on("SIGINT", () => shutdown("SIGINT", 0))
process.on("SIGTERM", () => shutdown("SIGTERM", 0))
