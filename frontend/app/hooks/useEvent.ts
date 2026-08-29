import { useQuery } from "@tanstack/react-query";
import { eventService } from "~/services/event.service";
import { getToken } from "~/lib/auth";
import type { EventInfo } from "~/types/event.types";

export const eventQueryKey = ["event", "current"] as const;

/**
 * The event, polled once a minute so the app flips from camera to album on its
 * own when the unlock moment passes — nobody has to reload mid-dinner.
 *
 * is_over comes from the server on every poll. The countdown rendered next to it
 * is cosmetic; this value is the gate.
 */
export function useEvent() {
  return useQuery<EventInfo>({
    queryKey: eventQueryKey,
    queryFn: () => eventService.getCurrent(),
    enabled: Boolean(getToken()),
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
    staleTime: 0,
  });
}
