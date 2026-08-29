import { useEffect, useState } from "react";
import { formatRemaining, remainingUntil } from "~/lib/countdown";

/**
 * Cosmetic only. When this reaches zero nothing happens locally — the app flips
 * to the album on the next poll of /events/current, because the server decides.
 */
export function Countdown({ endsAt }: { endsAt: string }) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  return <span>{formatRemaining(remainingUntil(endsAt, now))}</span>;
}
