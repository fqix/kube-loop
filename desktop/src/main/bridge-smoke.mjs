// Shell bridge smoke test.
//
// Exercises the whole desktop stack against a running app: the renderer's
// injected globals, the preload bridge, the main process IPC, and the Go
// sidecar behind it. Run the app with a debugging port and then this script:
//
//   npm run start -- --remote-debugging-port=9222
//   node electron/bridge-smoke.mjs
//
// It needs a machine with a display, so it is not part of `npm test`.
const target = await (async () => {
  for (let attempt = 0; attempt < 60; attempt++) {
    try {
      const pages = await fetch("http://127.0.0.1:9222/json/list").then(r => r.json());
      const page = pages.find(p => p.type === "page" && p.webSocketDebuggerUrl);
      if (page) return page;
    } catch {}
    await new Promise(r => setTimeout(r, 1000));
  }
  throw new Error("no renderer target appeared on the debugging port");
})();

const socket = new WebSocket(target.webSocketDebuggerUrl);
await new Promise(r => socket.addEventListener("open", r, { once: true }));

let nextId = 0;
const pending = new Map();
socket.addEventListener("message", event => {
  const message = JSON.parse(event.data);
  const waiter = pending.get(message.id);
  if (waiter) { pending.delete(message.id); waiter(message); }
});

function evaluate(expression) {
  const id = ++nextId;
  return new Promise(resolve => {
    pending.set(id, resolve);
    socket.send(JSON.stringify({
      id, method: "Runtime.evaluate",
      params: { expression, awaitPromise: true, returnByValue: true },
    }));
  });
}

// The renderer target appears before the preload has installed its globals,
// and the page may still navigate underneath us. Poll with short, independent
// evaluations so a destroyed execution context just retries.
async function waitForBridge(timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const reply = await evaluate("Boolean(window.go?.app?.App && window.runtime)");
    if (reply.result?.result?.value === true) return true;
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  return false;
}

if (!(await waitForBridge())) {
  console.log("FAIL  the desktop shell bridge never appeared on window");
  process.exit(1);
}

const checks = {
  "preload exposes window.go.app.App": "typeof window.go?.app?.App?.Bootstrap === 'function'",
  "preload exposes window.runtime.EventsOn": "typeof window.runtime?.EventsOn === 'function'",
  "Bootstrap crosses to Go": "window.go.app.App.Bootstrap().then(d => d.platform + '/' + d.coreVersion)",
  "ServerProfiles crosses to Go": "window.go.app.App.ServerProfiles().then(s => 'profiles=' + s.profiles.length)",
  "a Go error rejects the promise":
    "window.go.app.App.LoadServerInventory('missing','default').then(() => 'NO ERROR', e => 'rejected: ' + e.message.slice(0, 60))",
  "EventsOn returns an unsubscribe":
    "typeof window.runtime.EventsOn('update:state', () => {}) === 'function'",
  "window controls are wired": "window.runtime.WindowIsMaximised().then(v => 'maximised=' + v)",
  "React rendered the workspace":
    "new Promise(resolve => { const deadline = Date.now() + 20000; const tick = () => { const shell = document.querySelector('.desktop-shell'); if (shell && document.querySelectorAll('button').length > 3) resolve('buttons=' + document.querySelectorAll('button').length); else if (Date.now() > deadline) resolve(false); else setTimeout(tick, 200); }; tick(); })",
};

let failures = 0;
for (const [label, expression] of Object.entries(checks)) {
  const reply = await evaluate(expression);
  const result = reply.result?.result;
  const thrown = reply.result?.exceptionDetails;
  if (thrown || result?.value === false || result?.value === "NO ERROR") {
    failures++;
    console.log(`FAIL  ${label}: ${thrown?.exception?.description ?? JSON.stringify(result?.value)}`);
  } else {
    console.log(`ok    ${label}: ${JSON.stringify(result?.value)}`);
  }
}
socket.close();
console.log(failures === 0 ? "\nALL CHECKS PASSED" : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
