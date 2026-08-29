import { apiClient } from "~/lib/apiClient";
import type { EventInfo } from "~/types/event.types";

export const eventService = {
  getCurrent(): Promise<EventInfo> {
    return apiClient.get<EventInfo>("/events/current");
  },
};
