import { apiClient } from "~/lib/apiClient";
import type { PhotoListResponse, UploadResponse } from "~/types/event.types";

export const photoService = {
  /** Throws ApiError(423) while the event is still running. */
  list(): Promise<PhotoListResponse> {
    return apiClient.get<PhotoListResponse>("/events/current/photos");
  },

  /**
   * Uploads one capture. clientId is generated once per capture and reused on
   * every retry, which is what makes a lost response safe: the server answers
   * 200 with the original id instead of storing a second copy.
   */
  upload(input: {
    blob: Blob;
    clientId: string;
    takenAt: string;
    signal?: AbortSignal;
  }): Promise<UploadResponse> {
    const form = new FormData();
    form.append("file", input.blob, `${input.clientId}.jpg`);
    form.append("client_id", input.clientId);
    form.append("taken_at", input.takenAt);

    return apiClient.postForm<UploadResponse>("/events/current/photos", form, input.signal);
  },

  /** The server route that 302s to a presigned download. */
  originalUrl(photoId: string): string {
    return apiClient.url(`/photos/${photoId}/original`);
  },
};
