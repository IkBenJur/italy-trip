import { useEffect, useState } from "react";
import { uploader, type UploaderStatus } from "~/lib/uploader";

/**
 * Starts the background uploader for as long as the camera screen is mounted,
 * and reports how many captures are still waiting to go out.
 */
export function useUploader(): UploaderStatus & { dismissMessage: () => void } {
  const [status, setStatus] = useState<UploaderStatus>(() => uploader.getStatus());

  useEffect(() => {
    const unsubscribe = uploader.subscribe(setStatus);
    uploader.start();
    return () => {
      unsubscribe();
      uploader.stop();
    };
  }, []);

  return { ...status, dismissMessage: () => uploader.dismissMessage() };
}
