import { useOutletContext } from "react-router";
import type { EventInfo } from "~/types/event.types";

/** The event the gate already loaded, for the routes rendered inside it. */
export function useGateEvent(): EventInfo {
  return useOutletContext<EventInfo>();
}
