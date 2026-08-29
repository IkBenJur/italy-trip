import { describe, expect, it } from "vitest";
import { formatRemaining, remainingUntil } from "~/lib/countdown";

const endsAt = "2026-09-14T23:59:59+02:00";

describe("remainingUntil", () => {
  it("breaks the gap down into days, hours, minutes and seconds", () => {
    const now = new Date("2026-09-12T21:59:59+02:00");
    expect(remainingUntil(endsAt, now)).toMatchObject({
      days: 2,
      hours: 2,
      minutes: 0,
      seconds: 0,
    });
  });

  it("never goes negative once the moment has passed", () => {
    const remaining = remainingUntil(endsAt, new Date("2026-09-20T00:00:00+02:00"));
    expect(remaining.totalMs).toBe(0);
    expect(remaining).toMatchObject({ days: 0, hours: 0, minutes: 0, seconds: 0 });
  });

  it("compares instants, not wall clocks, across offsets", () => {
    // 21:59:59Z is exactly 23:59:59+02:00.
    expect(remainingUntil(endsAt, new Date("2026-09-14T21:59:59Z")).totalMs).toBe(0);
    expect(remainingUntil(endsAt, new Date("2026-09-14T21:59:58Z")).totalMs).toBe(1000);
  });
});

describe("formatRemaining", () => {
  it.each([
    ["2026-09-12T21:59:59+02:00", "2d 02h 00m"],
    ["2026-09-14T20:59:59+02:00", "3h 00m 00s"],
    ["2026-09-14T23:57:29+02:00", "2m 30s"],
    ["2026-09-15T00:00:00+02:00", "any moment now"],
  ])("formats the gap from %s as %s", (now, expected) => {
    expect(formatRemaining(remainingUntil(endsAt, new Date(now)))).toBe(expected);
  });
});
