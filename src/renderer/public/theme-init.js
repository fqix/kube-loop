// Applies the persisted theme before the first paint so the window does not
// flash the wrong scheme. Kept as a classic script (not inline) so the
// packaged app can ship a strict Content-Security-Policy.
(function () {
  try {
    var stored = localStorage.getItem("kube-loop.theme");
    var preference = stored === "dark" || stored === "system" || stored === "light" ? stored : "light";
    var dark =
      preference === "dark" ||
      (preference === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
    document.documentElement.classList.toggle("dark", dark);
    document.documentElement.style.colorScheme = dark ? "dark" : "light";
  } catch (e) {
    document.documentElement.classList.remove("dark");
    document.documentElement.style.colorScheme = "light";
  }
})();
