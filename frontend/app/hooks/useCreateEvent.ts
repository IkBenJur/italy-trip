import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eventService } from "~/services/event.service";
import { eventQueryKey } from "~/hooks/useEvent";
import type { CreateEventInput } from "~/types/event.types";

export function useCreateEvent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateEventInput) => eventService.create(input),
    onSuccess: (event) => {
      queryClient.setQueryData(eventQueryKey, event);
    },
  });
}
