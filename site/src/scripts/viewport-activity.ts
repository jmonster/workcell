interface ViewportActivityOptions {
  threshold?: number;
  pauseForReducedMotion?: boolean;
}

export const observeViewportActivity = (
  target: Element,
  onChange: (active: boolean) => void,
  options: ViewportActivityOptions = {},
) => {
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  const threshold = options.threshold ?? 0.01;
  const pauseForReducedMotion = options.pauseForReducedMotion ?? true;
  let intersecting = false;
  let lastState: boolean | undefined;

  const sync = () => {
    const active = intersecting
      && !document.hidden
      && (!pauseForReducedMotion || !reducedMotion.matches);
    if (active === lastState) return;
    lastState = active;
    onChange(active);
  };

  const observer = new IntersectionObserver(([entry]) => {
    intersecting = entry.isIntersecting;
    sync();
  }, { threshold });

  observer.observe(target);
  document.addEventListener('visibilitychange', sync);
  reducedMotion.addEventListener('change', sync);

  return () => {
    observer.disconnect();
    document.removeEventListener('visibilitychange', sync);
    reducedMotion.removeEventListener('change', sync);
  };
};
