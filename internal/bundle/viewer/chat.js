(() => {
  const PERSONA_CSS =
    "https://cdn.jsdelivr.net/npm/@runtypelabs/persona@4.17.0/dist/widget.css";
  const PERSONA_JS =
    "https://cdn.jsdelivr.net/npm/@runtypelabs/persona@4.17.0/dist/index.global.js";

  function loadPersonaStylesheet() {
    return new Promise((resolve, reject) => {
      // Persona Shadow DOM clones link[data-persona] from document.head.
      const existing = document.head.querySelector('link[data-persona]');
      if (existing) {
        resolve();
        return;
      }
      const link = document.createElement("link");
      link.rel = "stylesheet";
      link.href = PERSONA_CSS;
      link.setAttribute("data-persona", "");
      link.onload = () => resolve();
      link.onerror = () => reject(new Error("Failed to load Persona CSS"));
      document.head.appendChild(link);
    });
  }

  function loadScript(src) {
    return new Promise((resolve, reject) => {
      if (window.AgentWidget && typeof window.AgentWidget.initAgentWidget === "function") {
        resolve(window.AgentWidget);
        return;
      }
      const existing = document.querySelector(`script[src="${src}"]`);
      if (existing) {
        existing.addEventListener("load", () => resolve(window.AgentWidget));
        existing.addEventListener("error", () => reject(new Error("Failed to load Persona JS")));
        return;
      }
      const script = document.createElement("script");
      script.src = src;
      script.async = true;
      script.onload = () => resolve(window.AgentWidget);
      script.onerror = () => reject(new Error("Failed to load Persona JS"));
      document.head.appendChild(script);
    });
  }

  function currentDayNumber() {
    if (typeof window.tripmap?.getDayNumber === "function") {
      const n = Number(window.tripmap.getDayNumber());
      if (Number.isFinite(n) && n > 0) return n;
    }
    const trip = window.tripmap?.getTrip?.();
    const idx = window.tripmap?.getDayIndex?.() ?? 0;
    if (!trip?.days?.length) return 0;
    const i = Math.max(0, Math.min(trip.days.length - 1, Number(idx) || 0));
    const d = trip.days[i];
    const n = Number(d?.day);
    return Number.isFinite(n) && n > 0 ? n : i + 1;
  }

  async function mountPersona() {
    const root = document.getElementById("persona-root");
    if (!root) return;

    await loadPersonaStylesheet();
    const AW = await loadScript(PERSONA_JS);
    if (!AW?.initAgentWidget) {
      throw new Error("Persona initAgentWidget missing");
    }

    const apiUrl = new URL("api/chat", window.location.href).pathname;

    // Persona floating panel height is min(640, innerHeight-64) minus heightOffset.
    // Offset by 30% of that base so the open panel is ~70% of the default height.
    function floatingHeightOffset() {
      const base = Math.min(640, Math.max(200, window.innerHeight - 64));
      return Math.round(base * 0.3);
    }

    AW.initAgentWidget({
      target: root,
      useShadowDom: true,
      config: {
        apiUrl,
        // Disable Persona default starter chips ("Show me what you can help…").
        suggestionChips: [],
        launcher: {
          enabled: true,
          mountMode: "floating",
          position: "bottom-right",
          title: "Trip assistant",
          subtitle: "Nudge this itinerary",
          heightOffset: floatingHeightOffset(),
        },
        theme: {
          semantic: {
            colors: {
              accent: "#0f5c5c",
            },
          },
        },
        customFetch: async (url, init, payload) => {
          let base = payload && typeof payload === "object" ? payload : null;
          if (!base) {
            try {
              base = JSON.parse(init?.body || "{}");
            } catch {
              base = {};
            }
          }
          const day = currentDayNumber();
          const body = {
            ...base,
            day,
            context: {
              ...(base.context || {}),
              day,
              current_day: day,
            },
          };
          return fetch(url, {
            ...init,
            credentials: "include",
            headers: {
              ...(init.headers || {}),
              "Content-Type": "application/json",
            },
            body: JSON.stringify(body),
          });
        },
        parseSSEEvent: (data) => {
          // Never return null: Persona treats null as "not handled" and falls
          // through to the default parser (which paints junk into the transcript).
          // Return {} to swallow an event.
          if (!data || typeof data !== "object") return {};
          if (data.type === "trip_updated") {
            window.tripmap?.reloadTrip?.().catch((err) => console.error(err));
            return {};
          }
          if (data.type === "error" || data.error) {
            return { error: data.error || "Chat error" };
          }
          if (data.type === "done" || data.done) {
            return { done: true };
          }
          if (data.type === "status") {
            return {};
          }
          if (data.type === "text" && typeof data.text === "string" && data.text) {
            return { text: data.text };
          }
          return {};
        },
      },
    });
  }

  async function maybeEnableChat() {
    try {
      const res = await fetch("/auth/me", { credentials: "include" });
      if (!res.ok) return;
      const me = await res.json();
      if (!me.authenticated || !me.chat_enabled) return;
      await mountPersona();
      document.body.classList.add("chat-enabled");
    } catch (err) {
      console.warn("tripmap chat unavailable:", err);
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      maybeEnableChat();
    });
  } else {
    maybeEnableChat();
  }
})();
