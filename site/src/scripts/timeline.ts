export const createTimerGroup = () => {
  const timers = new Set<number>();
  const animationFrames = new Set<number>();

  const schedule = (callback: () => void, delay: number) => {
    const timer = window.setTimeout(() => {
      timers.delete(timer);
      callback();
    }, delay);
    timers.add(timer);
  };

  const frame = (callback: () => void) => {
    const animationFrame = window.requestAnimationFrame(() => {
      animationFrames.delete(animationFrame);
      callback();
    });
    animationFrames.add(animationFrame);
  };

  const clear = () => {
    timers.forEach((timer) => window.clearTimeout(timer));
    timers.clear();
    animationFrames.forEach((animationFrame) => window.cancelAnimationFrame(animationFrame));
    animationFrames.clear();
  };

  return { schedule, frame, clear };
};
