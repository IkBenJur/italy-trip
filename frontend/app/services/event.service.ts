import { apiClient } from "~/lib/apiClient";
import type { CreateEventInput, EventInfo } from "~/types/event.types";

export const eventService = {
  getCurrent(): Promise<EventInfo> {
    return apiClient.get<EventInfo>("/events/current");
  },

  create(input: CreateEventInput): Promise<EventInfo> {
    return apiClient.post<EventInfo>("/events", input);
  },
};
