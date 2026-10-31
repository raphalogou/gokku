import { useCallback, useRef, useState } from "react";
import { appendCapped, type ConsoleLine } from "~/lib/console";

/**
 * Console lines with pause: while paused, new lines wait in a buffer and the view does not
 * move; resuming shows them (LOG-2). The stream stays open either way.
 */
export function useConsoleBuffer() {
  const [lines, setLines] = useState<ConsoleLine[]>([]);
  const [paused, setPausedState] = useState(false);
  const [waiting, setWaiting] = useState(0);
  const pausedRef = useRef(false);
  const pending = useRef<ConsoleLine[]>([]);

  const push = useCallback((next: ConsoleLine[]) => {
    if (pausedRef.current) {
      pending.current = appendCapped(pending.current, next);
      setWaiting(pending.current.length);
    } else {
      setLines((prev) => appendCapped(prev, next));
    }
  }, []);

  const setPaused = useCallback((p: boolean) => {
    pausedRef.current = p;
    setPausedState(p);
    if (!p && pending.current.length > 0) {
      const flushed = pending.current;
      pending.current = [];
      setWaiting(0);
      setLines((prev) => appendCapped(prev, flushed));
    }
  }, []);

  const reset = useCallback((initial: ConsoleLine[] = []) => {
    pending.current = [];
    setWaiting(0);
    setLines(initial);
  }, []);

  return { lines, push, reset, paused, setPaused, waiting };
}
