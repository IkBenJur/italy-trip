/**
 * The countdown shown on the camera screen. Purely cosmetic: the app flips to
 * album mode when the server says so, not when this hits zero.
 */
export interface Remaining {
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
  totalMs: number;
}

export function remainingUntil(endsAt: string, now: Date = new Date()): Remaining {
  const totalMs = Math.max(0, new Date(endsAt).getTime() - now.getTime());
  const totalSeconds = Math.floor(totalMs / 1000);

  return {
    days: Math.floor(totalSeconds / 86_400),
    hours: Math.floor((totalSeconds % 86_400) / 3_600),
    minutes: Math.floor((totalSeconds % 3_600) / 60),
    seconds: totalSeconds % 60,
    totalMs,
  };
}

export function formatRemaining(remaining: Remaining): string {
  if (remaining.totalMs <= 0) return "any moment now";

  const pad = (value: number) => String(value).padStart(2, "0");

  if (remaining.days > 0) {
    return `${remaining.days}d ${pad(remaining.hours)}h ${pad(remaining.minutes)}m`;
  }
  if (remaining.hours > 0) {
    return `${remaining.hours}h ${pad(remaining.minutes)}m ${pad(remaining.seconds)}s`;
  }
  return `${remaining.minutes}m ${pad(remaining.seconds)}s`;
}
