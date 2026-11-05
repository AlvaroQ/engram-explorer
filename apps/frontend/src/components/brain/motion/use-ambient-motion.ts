/**
 * use-ambient-motion.ts — Whether the brain's ambient particle flow may run.
 *
 * Mount inside the R3F <Canvas>. Tracks tab visibility, whether the canvas is
 * on screen (IntersectionObserver) and prefers-reduced-motion changes, and
 * combines them with the caller's `animate` flag via ambientMotionActive().
 * When motion resumes it calls invalidate() once to restart the demand loop;
 * TweenDriver keeps it running from there.
 */
import { invalidate, useThree } from '@react-three/fiber';
import { useEffect, useState } from 'react';
import { ambientMotionActive } from './frame-demand.ts';
import { useReducedMotion } from './interpolator.ts';

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

export function useAmbientMotion(animate: boolean): boolean {
  const canvas = useThree((s) => s.gl.domElement);
  const [hidden, setHidden] = useState(() =>
    typeof document === 'undefined' ? false : document.hidden,
  );
  const [onScreen, setOnScreen] = useState(true);
  const [reducedMotion, setReducedMotion] = useState(useReducedMotion);

  useEffect(() => {
    const onVisibility = () => setHidden(document.hidden);
    document.addEventListener('visibilitychange', onVisibility);
    const mq = window.matchMedia(REDUCED_MOTION_QUERY);
    const onMotionPref = () => setReducedMotion(mq.matches);
    mq.addEventListener('change', onMotionPref);
    return () => {
      document.removeEventListener('visibilitychange', onVisibility);
      mq.removeEventListener('change', onMotionPref);
    };
  }, []);

  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') return;
    const io = new IntersectionObserver((entries) => {
      const entry = entries[entries.length - 1];
      if (entry) setOnScreen(entry.isIntersecting);
    });
    io.observe(canvas);
    return () => io.disconnect();
  }, [canvas]);

  const active = ambientMotionActive({ animate, reducedMotion, hidden, onScreen });

  // Restart the demand frameloop when ambient motion (re)starts.
  useEffect(() => {
    if (active) invalidate();
  }, [active]);

  return active;
}
