/** The event as the server describes it. Note there are no photo URLs here. */
export interface EventInfo {
  id: string;
  name: string;
  /** RFC3339 with offset. */
  starts_at: string;
  /** RFC3339 with offset. The single source of truth for the unlock moment. */
  ends_at: string;
  /**
   * Decided by the server, never by the browser's clock. Every gate in the app
   * routes off this and not off a local Date comparison.
   */
  is_over: boolean;
  /** Exposed while locked: a count is not content. */
  photo_count: number;
}

/** Input for starting a new event. Both are RFC3339 with an offset. */
export interface CreateEventInput {
  starts_at: string;
  ends_at: string;
}

/** A photo in the album. Only ever returned once the event is over. */
export interface Photo {
  id: string;
  taken_at: string;
  width: number;
  height: number;
  /** Presigned, 1h TTL. */
  url: string;
  /** Presigned, 1h TTL. */
  thumb_url: string;
}

export interface PhotoListResponse {
  photos: Photo[];
}

export interface UploadResponse {
  id: string;
  client_id: string;
  /** True when the server had already accepted this capture. */
  duplicate: boolean;
}
