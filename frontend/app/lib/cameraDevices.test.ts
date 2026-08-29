import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DEFAULT_FACING,
  hasMultipleCameras,
  readFacing,
  writeFacing,
  type Facing,
} from "./cameraDevices";

/** Installs a fake navigator.mediaDevices for the duration of one test. */
function stubMediaDevices(value: unknown) {
  const original = Object.getOwnPropertyDescriptor(navigator, "mediaDevices");
  Object.defineProperty(navigator, "mediaDevices", { value, configurable: true });
  return () => {
    if (original) {
      Object.defineProperty(navigator, "mediaDevices", original);
    } else {
      // @ts-expect-error removing a property jsdom did not define itself
      delete navigator.mediaDevices;
    }
  };
}

function videoinput(deviceId: string): MediaDeviceInfo {
  return {
    deviceId,
    kind: "videoinput",
    label: "",
    groupId: "",
    toJSON: () => ({}),
  } as MediaDeviceInfo;
}

function audioinput(deviceId: string): MediaDeviceInfo {
  return { ...videoinput(deviceId), kind: "audioinput" } as MediaDeviceInfo;
}

const restores: Array<() => void> = [];

afterEach(() => {
  while (restores.length) restores.pop()!();
});

function withMediaDevices(value: unknown) {
  restores.push(stubMediaDevices(value));
}

describe("hasMultipleCameras", () => {
  it("is false when mediaDevices is missing entirely", async () => {
    withMediaDevices(undefined);
    await expect(hasMultipleCameras()).resolves.toBe(false);
  });

  it("is false when the browser has no enumerateDevices", async () => {
    withMediaDevices({});
    await expect(hasMultipleCameras()).resolves.toBe(false);
  });

  it("is false when enumeration rejects", async () => {
    withMediaDevices({ enumerateDevices: vi.fn().mockRejectedValue(new Error("nope")) });
    await expect(hasMultipleCameras()).resolves.toBe(false);
  });

  it("is false for a single camera", async () => {
    withMediaDevices({
      enumerateDevices: vi.fn().mockResolvedValue([videoinput("front"), audioinput("mic")]),
    });
    await expect(hasMultipleCameras()).resolves.toBe(false);
  });

  it("is false when there are no cameras at all", async () => {
    withMediaDevices({ enumerateDevices: vi.fn().mockResolvedValue([audioinput("mic")]) });
    await expect(hasMultipleCameras()).resolves.toBe(false);
  });

  it("is true once a second camera is enumerated", async () => {
    withMediaDevices({
      enumerateDevices: vi
        .fn()
        .mockResolvedValue([videoinput("front"), videoinput("rear"), audioinput("mic")]),
    });
    await expect(hasMultipleCameras()).resolves.toBe(true);
  });
});

describe("facing preference", () => {
  it("defaults to the rear camera on a device we have not seen", () => {
    expect(readFacing()).toBe(DEFAULT_FACING);
    expect(DEFAULT_FACING).toBe("environment");
  });

  it("round-trips both cameras", () => {
    for (const facing of ["user", "environment"] as Facing[]) {
      writeFacing(facing);
      expect(readFacing()).toBe(facing);
    }
  });

  it("falls back to the default when the stored value is garbage", () => {
    window.localStorage.setItem("camera_facing", "sideways");
    expect(readFacing()).toBe(DEFAULT_FACING);
  });

  it("survives a storage that throws on write", () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    expect(() => writeFacing("user")).not.toThrow();
    setItem.mockRestore();
  });

  it("survives a storage that throws on read", () => {
    const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(readFacing()).toBe(DEFAULT_FACING);
    getItem.mockRestore();
  });
});
