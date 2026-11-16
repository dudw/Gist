import { useRegisterSW } from "virtual:pwa-register/react";

const UPDATE_CHECK_INTERVAL = 60 * 60 * 1000; // 1 hour

/**
 * 每 1 小时主动拉取一次 sw.js（绕过 HTTP 缓存）并触发注册更新检查。
 *
 * 浏览器只在导航时自动检查 sw.js，单页应用可能长时间不导航，
 * 因此需要定时检查，否则新版本要等到下一次刷新才被发现。
 */
function registerPeriodicSync(
  period: number,
  swUrl: string,
  registration: ServiceWorkerRegistration,
) {
  if (period <= 0) return;

  setInterval(async () => {
    if ("onLine" in navigator && !navigator.onLine) return;

    const resp = await fetch(swUrl, {
      cache: "no-store",
      headers: {
        cache: "no-store",
        "cache-control": "no-cache",
      },
    });

    if (resp?.status === 200) await registration.update();
  }, period);
}

/**
 * 注册 Service Worker 并定时检查更新。
 *
 * registerType 为 "autoUpdate"：构建时 sw.js 自带 skipWaiting + clientsClaim，
 * 新版本安装后立即接管，workbox-window 在 activated(isUpdate) 时自动刷新页面，
 * 因此不需要「发现新版本」提示 UI。
 */
export function ServiceWorkerRegistrar() {
  useRegisterSW({
    onRegisteredSW(swUrl, registration) {
      if (UPDATE_CHECK_INTERVAL <= 0) return;
      if (registration?.active?.state === "activated") {
        registerPeriodicSync(UPDATE_CHECK_INTERVAL, swUrl, registration);
      } else if (registration?.installing) {
        registration.installing.addEventListener("statechange", (event) => {
          const sw = event.target as ServiceWorker;
          if (sw.state === "activated")
            registerPeriodicSync(UPDATE_CHECK_INTERVAL, swUrl, registration);
        });
      }
    },
  });

  return null;
}
