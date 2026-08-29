import { useQuery } from "@tanstack/react-query";
import { photoService } from "~/services/photo.service";
import { getToken } from "~/lib/auth";
import type { PhotoListResponse } from "~/types/event.types";

export const photosQueryKey = ["photos", "list"] as const;

/**
 * The album. Only ever enabled once the server says the event is over — asking
 * earlier just earns a 423, and the presigned URLs it returns expire after an
 * hour, so they are refetched rather than cached indefinitely.
 */
export function usePhotos(enabled: boolean) {
  return useQuery<PhotoListResponse>({
    queryKey: photosQueryKey,
    queryFn: () => photoService.list(),
    enabled: enabled && Boolean(getToken()),
    staleTime: 30 * 60_000,
    refetchOnWindowFocus: false,
  });
}
