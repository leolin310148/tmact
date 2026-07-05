import { useCallback, useEffect, useState } from "react";

import { loadVAPIDPublicKey, subscribePush, unsubscribePush } from "../api/client";

export type PushNotificationState =
  | "checking"
  | "unsupported"
  | "not-configured"
  | "blocked"
  | "unsubscribed"
  | "subscribed"
  | "busy"
  | "error";

export interface UsePushNotificationsResult {
  state: PushNotificationState;
  message: string;
  enable: () => Promise<void>;
  disable: () => Promise<void>;
}

export function usePushNotifications(): UsePushNotificationsResult {
  const [state, setState] = useState<PushNotificationState>("checking");
  const [message, setMessage] = useState("Checking notification support...");
  const [registration, setRegistration] = useState<ServiceWorkerRegistration | null>(null);
  const [publicKey, setPublicKey] = useState("");

  const refreshSubscriptionState = useCallback(async (reg: ServiceWorkerRegistration) => {
    const sub = await reg.pushManager.getSubscription();
    if (sub) {
      setState("subscribed");
      setMessage("Notifications enabled for this browser.");
    } else {
      setState("unsubscribed");
      setMessage("Notifications are available.");
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function init() {
      if (!("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
        if (!cancelled) {
          setState("unsupported");
          setMessage("Web Push is not supported in this browser.");
        }
        return;
      }
      if (Notification.permission === "denied") {
        if (!cancelled) {
          setState("blocked");
          setMessage("Notification permission is blocked in browser settings.");
        }
        return;
      }

      try {
        const keyRes = await loadVAPIDPublicKey();
        if (!keyRes.res.ok || !keyRes.data.publicKey) {
          if (!cancelled) {
            setState("not-configured");
            setMessage("Server VAPID key is not configured.");
          }
          return;
        }

        await navigator.serviceWorker.register("/sw.js");
        const reg = await navigator.serviceWorker.ready;
        if (cancelled) return;
        setPublicKey(keyRes.data.publicKey);
        setRegistration(reg);
        await refreshSubscriptionState(reg);
      } catch (err) {
        if (!cancelled) {
          setState("error");
          setMessage(errorMessage(err));
        }
      }
    }

    void init();
    return () => {
      cancelled = true;
    };
  }, [refreshSubscriptionState]);

  const enable = useCallback(async () => {
    if (!registration || !publicKey) return;
    setState("busy");
    setMessage("Enabling notifications...");
    let subscription: PushSubscription | null = null;
    try {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(publicKey),
      });
      const res = await subscribePush(subscription);
      if (!res.res.ok) {
        throw new Error(responseError(res.data, res.res));
      }
      setState("subscribed");
      setMessage("Notifications enabled for this browser.");
    } catch (err) {
      if (subscription) {
        await subscription.unsubscribe().catch(() => {});
      }
      if ("Notification" in window && Notification.permission === "denied") {
        setState("blocked");
        setMessage("Notification permission is blocked in browser settings.");
      } else {
        setState("error");
        setMessage(errorMessage(err));
      }
    }
  }, [publicKey, registration]);

  const disable = useCallback(async () => {
    if (!registration) return;
    setState("busy");
    setMessage("Disabling notifications...");
    try {
      const sub = await registration.pushManager.getSubscription();
      if (sub) {
        const res = await unsubscribePush(sub.endpoint);
        if (!res.res.ok) {
          throw new Error(responseError(res.data, res.res));
        }
        await sub.unsubscribe();
      }
      setState("unsubscribed");
      setMessage("Notifications are available.");
    } catch (err) {
      setState("error");
      setMessage(errorMessage(err));
    }
  }, [registration]);

  return { state, message, enable, disable };
}

function urlBase64ToUint8Array(base64String: string): ArrayBuffer {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const rawData = window.atob(base64);
  const buffer = new ArrayBuffer(rawData.length);
  const outputArray = new Uint8Array(buffer);
  for (let i = 0; i < rawData.length; i += 1) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return buffer;
}

function responseError(data: unknown, res: Response): string {
  if (data && typeof data === "object" && "error" in data && typeof data.error === "string") {
    return data.error;
  }
  return res.statusText || `HTTP ${res.status}`;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return "Notification setup failed.";
}
