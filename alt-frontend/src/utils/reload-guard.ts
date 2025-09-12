/**
 * 一回限りのリロードガード
 * ChunkLoadErrorやその他のエラーでの永続リロードを防ぐ
 */
export function reloadOnce(key = "alt:reload-once") {
  if (typeof window === "undefined") return;

  if (sessionStorage.getItem(key)) {
    console.warn(
      "[ReloadGuard] リロードは既に実行されています。無限ループを防ぐため、再リロードをスキップします。",
    );
    return;
  }

  sessionStorage.setItem(key, "1");
  console.log("[ReloadGuard] ページをリロードします（一回限り）");
  location.reload();
}

/**
 * ChunkLoadErrorかどうかを判定
 */
export function isChunkLoadError(error: Error | ErrorEvent): boolean {
  if (error instanceof Error) {
    return (
      error.name === "ChunkLoadError" ||
      error.message.includes("Loading chunk") ||
      error.message.includes("Loading CSS chunk")
    );
  }

  if ("error" in error && error.error instanceof Error) {
    return isChunkLoadError(error.error);
  }

  return false;
}

/**
 * グローバルエラーハンドリングの設定
 */
export function setupErrorHandling() {
  if (typeof window === "undefined") return;

  window.addEventListener("error", (event) => {
    if (isChunkLoadError(event)) {
      console.warn("[ReloadGuard] ChunkLoadErrorを検出しました");
      reloadOnce();
    }
  });

  window.addEventListener("unhandledrejection", (event) => {
    if (event.reason && isChunkLoadError(event.reason)) {
      console.warn(
        "[ReloadGuard] ChunkLoadError (Promise rejection)を検出しました",
      );
      reloadOnce();
    }
  });
}
