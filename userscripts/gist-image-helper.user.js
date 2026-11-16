// ==UserScript==
// @name         Gist 图片直连助手
// @namespace    https://gist.godof.uk/
// @version      2.0.1
// @description  接管 gist.godof.uk 中加载失败的图片；直连失败时借助图片源站标签页的同源 Cookie 重新拉取，绕过 Cloudflare 质询、防盗链、CORS/CORP 与混合内容限制
// @author       gist
// @match        https://gist.godof.uk/*
// @match        https://www.141ppv.com/*
// @grant        GM_xmlhttpRequest
// @grant        GM.xmlHttpRequest
// @grant        GM_cookie
// @grant        GM.cookie
// @grant        GM_setValue
// @grant        GM_getValue
// @grant        GM_listValues
// @grant        GM_deleteValue
// @grant        GM_addValueChangeListener
// @grant        GM_removeValueChangeListener
// @grant        GM_openInTab
// @grant        GM.setValue
// @grant        GM.getValue
// @grant        GM.listValues
// @grant        GM.deleteValue
// @grant        GM.addValueChangeListener
// @grant        GM.removeValueChangeListener
// @grant        GM.openInTab
// @connect      *
// @run-at       document-idle
// ==/UserScript==

/**
 * Gist 图片直连助手
 *
 * 背景：Gist 前端已经把图片改为浏览器直连，并按 no-referrer → origin 两种策略重试。
 * 但部分站点会同时校验完整文章地址作为 Referer，或直接禁止跨域/混合内容加载，
 * 页面最终只能显示「图片加载失败 / 打开原图」占位。
 *
 * 本脚本分两个页面协作（因此除 gist.godof.uk 外，还需匹配图片源站）：
 *
 * A. gist 页：扫描「打开原图」占位，先用 GM 直连；失败则在后台打开图片标签页并等待回传。
 * B. 图片源站标签页：同源环境下浏览器会自动带上 cf_clearance，
 *    用 GM_xmlhttpRequest 读取图片本体，经 GM 存储回传给 gist 页渲染。
 *
 * 为什么需要 B 页：Cloudflare 的 cf_clearance 是 HttpOnly + SameSite=Lax，
 * 从 gist 页跨站调用 GM_cookie.list 读不到，显式注入 Cookie 头也会被浏览器过滤。
 * 只有当请求发生在图片源站自身的标签页里时，cookie 才会被自动携带。
 *
 * 注意：GM_xmlhttpRequest 仍走本机网络，无法突破对图片域名的网络封锁。
 */

(function () {
  "use strict";

  /** 前端错误占位的结构特征：带虚线边框的 span 容器 */
  const PLACEHOLDER_SELECTOR = 'span[class*="border-dashed"]';
  /** 「打开原图」在 zh/en 两种语言下的文案 */
  const OPEN_ORIGINAL_TEXT = /^(打开原图|Open original image)$/i;
  /** 同时进行中的 GM 请求数量 */
  const MAX_CONCURRENCY = 3;
  /** 单个请求超时（毫秒） */
  const TIMEOUT_MS = 20000;
  /** 双页协作等待超时（毫秒） */
  const TAB_HANDSHAKE_TIMEOUT_MS = 30000;
  /** gist 页轮询 GM 存储的间隔（毫秒） */
  const POLL_INTERVAL_MS = 400;
  /** 当前是否在 gist 阅读页；另一个分支是图片源站的标签页 */
  const isGist = window.location.hostname === "gist.godof.uk";

  /**
   * 统一取 GM API：优先新版 GM.*，回退旧式全局函数。
   * 两种写法都已在 @grant 中声明，脚本管理器才会注入。
   */
  function gmApi(name) {
    if (typeof GM !== "undefined" && typeof GM[name] === "function") {
      return GM[name].bind(GM);
    }
    if (typeof globalThis[name] === "function") {
      return globalThis[name].bind(globalThis);
    }
    return null;
  }

  const gmRequest =
    typeof GM_xmlhttpRequest === "function"
      ? GM_xmlhttpRequest
      : typeof GM !== "undefined" && typeof GM.xmlHttpRequest === "function"
        ? GM.xmlHttpRequest.bind(GM)
        : null;

  if (!gmRequest) {
    console.warn("[gist-image-helper] 当前脚本管理器不支持 GM_xmlhttpRequest，已退出");
    return;
  }

  /** 已入队的链接，避免 MutationObserver 反复处理 */
  const handled = new WeakSet();
  /** 按 entryId 缓存条目原文地址的请求 */
  const entryUrlCache = new Map();
  /** 图片 URL → Blob 请求，避免同一张图重复下载 */
  const blobCache = new Map();
  /** 待处理任务队列 */
  const queue = [];
  let active = 0;

  /**
   * 从当前路由解析 entryId。
   * 路由格式见 frontend/src/lib/router.ts：
   * /all/:id、/starred/:id、/search/:id、/feed/:feedId/:id、/folder/:folderId/:id
   */
  function entryIdFromPath() {
    const segments = window.location.pathname.split("/").filter(Boolean);
    const [first, second, third] = segments;
    if (first === "all" || first === "starred" || first === "search") {
      return second || null;
    }
    if (first === "feed" || first === "folder") {
      return third || null;
    }
    return null;
  }

  /** 通过后端接口取文章原始地址，结果按 entryId 缓存 */
  function entryUrlFromApi(entryId) {
    if (!entryUrlCache.has(entryId)) {
      const task = fetch(`/api/entries/${encodeURIComponent(entryId)}`, {
        credentials: "include",
      })
        .then((response) => (response.ok ? response.json() : null))
        .then((entry) =>
          entry && typeof entry.url === "string" ? entry.url : null,
        )
        .catch(() => null);
      entryUrlCache.set(entryId, task);
    }
    return entryUrlCache.get(entryId);
  }

  /**
   * 取当前文章的原始地址：
   * 优先用正文标题上的外链（同一页面只有一篇正文），取不到再查接口。
   */
  function resolveArticleUrl() {
    const titleLink = document.querySelector('h1 a[href^="http"]');
    if (titleLink) return Promise.resolve(titleLink.href);

    const entryId = entryIdFromPath();
    if (!entryId) return Promise.resolve(null);
    return entryUrlFromApi(entryId);
  }

  /**
   * 读取目标域 Cookie（含 HttpOnly 的 cf_clearance）。
   * 跨站 GM 请求不会自动携带 SameSite=Lax 的 Cloudflare 验证 Cookie，必须显式注入。
   * 兼容 TM 的回调风格与 VM 的 Promise 风格。
   */
  function getCookieInfo(url) {
    return new Promise((resolve) => {
      const api =
        typeof GM_cookie !== "undefined"
          ? GM_cookie
          : typeof GM !== "undefined"
            ? GM.cookie
            : undefined;
      if (!api || typeof api.list !== "function") {
        resolve({ api: "none", header: "", names: [] });
        return;
      }

      let settled = false;
      const finish = (cookies) => {
        if (settled) return;
        settled = true;
        const list = cookies || [];
        resolve({
          api: "available",
          header: list.map((c) => `${c.name}=${c.value}`).join("; "),
          names: list.map((c) => c.name),
        });
      };

      try {
        const result = api.list({ url }, finish);
        if (result && typeof result.then === "function") {
          result.then(finish, () => finish([]));
        }
      } catch {
        finish([]);
      }
    });
  }

  let cookieDiagnosed = false;
  let errorBodyDiagnosed = false;

  /** 用 GM 请求取图片 Blob，referer 为空表示不发送 Referer */
  async function requestBlob(url, referer) {
    const { header: cookieHeader, api, names } = await getCookieInfo(url);
    if (!cookieDiagnosed) {
      cookieDiagnosed = true;
      console.info("[gist-image-helper] Cookie 诊断", {
        api,
        names,
        length: cookieHeader.length,
      });
    }
    return new Promise((resolve, reject) => {
      const headers = {
        Accept: "image/avif,image/webp,image/apng,image/*,*/*;q=0.8",
        "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        "Sec-Fetch-Dest": "image",
        "Sec-Fetch-Mode": "no-cors",
        "Sec-Fetch-Site": "cross-site",
        "User-Agent": navigator.userAgent,
      };
      if (referer) headers.Referer = referer;
      if (cookieHeader) headers.Cookie = cookieHeader;

      gmRequest({
        method: "GET",
        url,
        headers,
        // TM 特有的 cookie 参数，部分管理器只认这个而不认 headers.Cookie
        cookie: cookieHeader || undefined,
        responseType: "blob",
        timeout: TIMEOUT_MS,
        onload(response) {
          if (response.status < 200 || response.status >= 300) {
            if (!errorBodyDiagnosed) {
              errorBodyDiagnosed = true;
              console.debug(
                "[gist-image-helper] 错误响应体",
                (response.responseText || "").slice(0, 300),
              );
            }
            reject(new Error(`HTTP ${response.status}`));
            return;
          }

          const blob = response.response;
          // 不同脚本管理器的 Blob 可能来自不同 realm，不能用 instanceof 判断
          if (
            !blob ||
            typeof blob.size !== "number" ||
            typeof blob.slice !== "function"
          ) {
            reject(new Error("invalid blob response"));
            return;
          }
          if (blob.size === 0) {
            reject(new Error("empty response"));
            return;
          }
          if (
            blob.type &&
            !/^image\//i.test(blob.type) &&
            blob.type !== "application/octet-stream"
          ) {
            reject(new Error(`not an image: ${blob.type}`));
            return;
          }
          resolve(blob);
        },
        onerror: () => reject(new Error("network error")),
        ontimeout: () => reject(new Error("timeout")),
        onabort: () => reject(new Error("aborted")),
      });
    });
  }

  /** 根据文章地址和图片路径构造多档 Referer 候选 */
  function buildRefererCandidates(url, articleUrl) {
    const referers = [];

    if (articleUrl) {
      referers.push(articleUrl);
      try {
        const parsed = new URL(articleUrl);
        if (parsed.search) {
          const noQuery = new URL(parsed.href);
          noQuery.search = "";
          referers.push(noQuery.href);
        }
        referers.push(`${parsed.origin}/`);
      } catch {
        // 非法文章地址忽略
      }
    }

    // 从图片路径推导同站详情页：/movies/PPV5008836.jpg → /movies/PPV5008836.html
    try {
      const image = new URL(url);
      const match = image.pathname.match(
        /^(.*\/)([^/]+)\.(?:jpe?g|png|webp|gif)$/i,
      );
      if (match) {
        referers.push(`${image.origin}${match[1]}${match[2]}.html`);
      }
      referers.push(`${image.origin}/`);
    } catch {
      // 非法图片地址忽略
    }

    referers.push(null);
    return [...new Set(referers)];
  }

  /** 依次尝试各档 Referer，全部失败才抛错 */
  async function fetchWithFallback(url, articleUrl) {
    const referers = buildRefererCandidates(url, articleUrl);

    let lastError = null;
    for (const referer of referers) {
      try {
        return await requestBlob(url, referer);
      } catch (error) {
        lastError = error;
        console.debug("[gist-image-helper] Referer 尝试失败", {
          referer: referer ?? "(none)",
          reason: error.message,
        });
      }
    }
    throw lastError ?? new Error("all referer strategies failed");
  }

  /** 同一张图只发起一次请求，失败后允许下次重试 */
  function loadBlob(imageUrl) {
    if (!blobCache.has(imageUrl)) {
      const task = (async () => {
        const articleUrl = await resolveArticleUrl();
        return fetchWithFallback(imageUrl, articleUrl);
      })().catch((error) => {
        blobCache.delete(imageUrl);
        throw error;
      });
      blobCache.set(imageUrl, task);
    }
    return blobCache.get(imageUrl);
  }

  /** 把占位容器替换成真正的图片，保留外层节点身份避免干扰 React */
  function renderImage(link, imageUrl, blob) {
    const box = link.closest(PLACEHOLDER_SELECTOR);
    if (!box) return;

    const objectUrl = URL.createObjectURL(blob);
    const image = document.createElement("img");
    image.src = objectUrl;
    image.alt = "";
    image.loading = "lazy";
    image.decoding = "async";
    image.referrerPolicy = "no-referrer";
    image.title = "由油猴脚本加载";
    image.style.cssText =
      "display:block;max-width:100%;height:auto;margin:0 auto;border-radius:0.25rem;cursor:zoom-in;";
    image.addEventListener("click", () => {
      window.open(imageUrl, "_blank", "noopener,noreferrer");
    });

    // 保留 span 本体，只替换子节点；React 重渲染时最多恢复原占位，不会报错
    box.replaceChildren(image);
    box.style.display = "block";
    box.style.padding = "0";
    box.style.border = "0";
    box.style.background = "transparent";
    box.dataset.gmImage = "loaded";

    // 页面关闭时释放 blob，避免长时间挂着内存
    window.addEventListener(
      "pagehide",
      () => URL.revokeObjectURL(objectUrl),
      { once: true },
    );
  }

  // ── 双页协作：gist 页发起请求并等待图片源站标签页回传 ──────────────────

  /** gist 页：打开图片源站标签页，轮询 GM 存储等待回传 */
  async function requestViaImageTab(imageUrl) {
    const setValue = gmApi("setValue");
    const getValue = gmApi("getValue");
    const deleteValue = gmApi("deleteValue");
    const openInTab = gmApi("openInTab");

    if (!setValue || !getValue || !openInTab) {
      return Promise.reject(new Error("当前脚本管理器缺少双页协作所需的 GM API"));
    }

    // 图片文档（image/jpeg）不保证能注入用户脚本，优先打开同源的 HTML 文章页
    const articleUrl = await resolveArticleUrl();
    const landingUrl = articleUrl || imageUrl;

    return new Promise((resolve, reject) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const reqKey = `gist-image-req-${id}`;
      const resKey = `gist-image-res-${id}`;
      let done = false;

      const timer = setTimeout(
        () => finish(new Error("图片标签页响应超时（请确认源站页面脚本已注入）")),
        TAB_HANDSHAKE_TIMEOUT_MS,
      );

      // 轮询而非依赖 addValueChangeListener：跨标签事件在部分管理器中不可靠
      const poller = setInterval(() => {
        const value = getValue(resKey);
        if (!value) return;
        if (value === "__ERROR__") {
          finish(new Error("图片标签页加载失败"));
          return;
        }
        try {
          // 回传的是完整 data URL，保留服务端返回的图片 MIME
          const matched = /^data:([^;]+);base64,(.+)$/.exec(value);
          if (!matched) throw new Error("无效的图片数据");
          const binary = atob(matched[2]);
          const bytes = new Uint8Array(binary.length);
          for (let i = 0; i < binary.length; i += 1) {
            bytes[i] = binary.charCodeAt(i);
          }
          finish(null, new Blob([bytes], { type: matched[1] }));
        } catch (error) {
          finish(error);
        }
      }, POLL_INTERVAL_MS);

      function cleanup() {
        clearTimeout(timer);
        clearInterval(poller);
        try {
          deleteValue?.(reqKey);
          deleteValue?.(resKey);
        } catch {
          // 存储已清理，忽略
        }
      }

      function finish(error, blob) {
        if (done) return;
        done = true;
        cleanup();
        if (error) reject(error);
        else resolve(blob);
      }

      setValue(reqKey, JSON.stringify({ imageUrl, landingUrl, resKey }));
      openInTab(landingUrl, { active: false, insert: true });
      console.debug("[gist-image-helper] 已打开源站标签页，等待回传", landingUrl);
    });
  }

  // ── 图片源站标签页：同源请求图片本体并回传 ────────────────────────────

  /** 在图片源站页面请求自身，同源请求会自动携带 cf_clearance */
  function requestSameOriginBlob(url) {
    return new Promise((resolve, reject) => {
      gmRequest({
        method: "GET",
        url,
        responseType: "blob",
        timeout: TIMEOUT_MS,
        onload(response) {
          if (response.status >= 200 && response.status < 300) {
            resolve(response.response);
          } else {
            reject(new Error(`HTTP ${response.status}`));
          }
        },
        onerror: () => reject(new Error("network error")),
        ontimeout: () => reject(new Error("timeout")),
      });
    });
  }

  /** Blob → 完整 data URL（含 MIME），便于通过 GM_setValue 跨标签页传递 */
  function blobToDataUrl(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(blob);
    });
  }

  /**
   * 图片标签页入口：认领一个待处理请求，同源取回图片后把结果写入 GM 存储。
   * 不要求当前 URL 与请求 URL 精确匹配——任意源站页面都能同源取图，
   * 避免因重定向、查询参数差异导致匹配失败而「打开后没反应」。
   */
  async function imageTabMain() {
    const getValue = gmApi("getValue");
    const setValue = gmApi("setValue");
    const listValues = gmApi("listValues");
    const deleteValue = gmApi("deleteValue");
    if (!getValue || !setValue || !listValues || !deleteValue) {
      console.warn("[gist-image-helper] 图片标签页缺少 GM 存储 API，无法回传");
      return;
    }

    console.info("[gist-image-helper] 源站标签页脚本已启动", location.href);
    const keys = listValues().filter((key) =>
      key.startsWith("gist-image-req-"),
    );
    console.info("[gist-image-helper] 待处理请求数", keys.length);

    for (const key of keys) {
      // 认领机制：避免多个源站标签页重复处理同一个请求
      const claimKey = `${key}:claimed`;
      if (getValue(claimKey)) continue;
      setValue(claimKey, "1");

      let request;
      try {
        request = JSON.parse(getValue(key));
      } catch {
        continue;
      }
      if (!request?.imageUrl) continue;

      console.info("[gist-image-helper] 源站标签页开始取图", request.imageUrl);
      try {
        // 请求图片本体：此时是源站标签页的同源请求，会自动携带 cf_clearance
        const blob = await requestSameOriginBlob(request.imageUrl);
        const dataUrl = await blobToDataUrl(blob);
        setValue(request.resKey, dataUrl);
        console.info("[gist-image-helper] 源站标签页已回传图片");
      } catch (error) {
        console.warn("[gist-image-helper] 源站标签页取图失败", error);
        setValue(request.resKey, "__ERROR__");
      } finally {
        deleteValue(key);
        deleteValue(claimKey);
        try {
          window.close();
        } catch {
          // 浏览器可能禁止关闭，保留标签页由用户手动关闭
        }
      }
      return;
    }
    console.info("[gist-image-helper] 源站标签页未找到待处理请求");
  }

  /** 同一张图只开一个源站标签页，避免重复渲染时反复弹出标签 */
  const tabRequestCache = new Map();

  function requestViaImageTabOnce(imageUrl) {
    if (!tabRequestCache.has(imageUrl)) {
      tabRequestCache.set(imageUrl, requestViaImageTab(imageUrl));
    }
    return tabRequestCache.get(imageUrl);
  }

  async function processTask(link, imageUrl) {
    try {
      let blob;
      try {
        blob = await loadBlob(imageUrl);
      } catch (directError) {
        console.debug(
          "[gist-image-helper] 直连失败，改用图片标签页协作",
          directError,
        );
        blob = await requestViaImageTabOnce(imageUrl);
      }
      renderImage(link, imageUrl, blob);
      console.debug("[gist-image-helper] 已加载", imageUrl);
    } catch (error) {
      link.dataset.gmImage = "failed";
      console.debug("[gist-image-helper] 加载失败", imageUrl, error);
    }
  }

  function pump() {
    while (active < MAX_CONCURRENCY && queue.length > 0) {
      const task = queue.shift();
      active += 1;
      processTask(task.link, task.imageUrl).finally(() => {
        active -= 1;
        pump();
      });
    }
  }

  /** 只处理占位里文案为「打开原图」的那个链接 */
  function enqueue(link) {
    if (handled.has(link)) return;

    const text = (link.textContent || "").trim();
    if (!OPEN_ORIGINAL_TEXT.test(text)) return;

    const imageUrl = link.getAttribute("href");
    if (!imageUrl || !/^https?:\/\//i.test(imageUrl)) return;

    handled.add(link);
    queue.push({ link, imageUrl });
    pump();
  }

  function scan(root) {
    if (root.nodeType !== 1) return;

    if (root.matches?.(`${PLACEHOLDER_SELECTOR} a[href]`)) {
      enqueue(root);
    }
    root
      .querySelectorAll?.(`${PLACEHOLDER_SELECTOR} a[href]`)
      .forEach((link) => enqueue(link));
  }

  // MutationObserver 可能一次回调大量节点，合并到下一帧统一扫描
  let scheduled = false;
  let pendingRoots = [];

  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === 1) pendingRoots.push(node);
      }
    }

    if (scheduled || pendingRoots.length === 0) return;
    scheduled = true;
    requestAnimationFrame(() => {
      scheduled = false;
      const roots = pendingRoots;
      pendingRoots = [];
      for (const root of roots) scan(root);
    });
  });

  if (isGist) {
    observer.observe(document.body, { childList: true, subtree: true });
    scan(document.body);
  } else {
    // 图片源站标签页：等待 gist 页发起的协作请求
    imageTabMain();
  }
})();
