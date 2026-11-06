export function bindTaskPollingLifecycle({ documentRef, windowRef, onPause, onResume }) {
  let paused = false;

  const pauseOnce = () => {
    if (paused) return;
    paused = true;
    onPause();
  };

  const resumeOnce = () => {
    if (!paused) return;
    paused = false;
    onResume();
  };

  const handleVisibilityChange = () => {
    if (documentRef.hidden) pauseOnce();
    else resumeOnce();
  };

  const handlePageHide = (event) => {
    if (event?.persisted) pauseOnce();
  };

  const handlePageShow = (event) => {
    if (event?.persisted && !documentRef.hidden) resumeOnce();
  };

  documentRef.addEventListener("visibilitychange", handleVisibilityChange);
  windowRef.addEventListener("pagehide", handlePageHide);
  windowRef.addEventListener("pageshow", handlePageShow);

  return () => {
    documentRef.removeEventListener("visibilitychange", handleVisibilityChange);
    windowRef.removeEventListener("pagehide", handlePageHide);
    windowRef.removeEventListener("pageshow", handlePageShow);
  };
}
