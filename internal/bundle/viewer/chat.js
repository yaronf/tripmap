(() => {
  const PERSONA_CSS =
    "https://cdn.jsdelivr.net/npm/@runtypelabs/persona@4.17.0/dist/widget.css";
  const PERSONA_JS =
    "https://cdn.jsdelivr.net/npm/@runtypelabs/persona@4.17.0/dist/index.global.js";

  function loadPersonaStylesheet() {
    return new Promise((resolve, reject) => {
      // Persona Shadow DOM clones link[data-persona] from document.head.
      const existing = document.head.querySelector("link[data-persona]");
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

  // Persona hard-codes header py-5 / footer py-4 utility classes; theme
  // padding tokens do not override them. Inject compact rules into each
  // ShadowRoot (Persona only clones the first link[data-persona]).
  const COMPACT_CSS = `
    .persona-widget-header {
      padding: 0.35rem 0.65rem !important;
      gap: 0.45rem !important;
      min-height: 0 !important;
    }
    .persona-widget-footer:not(.persona-widget-footer--pill) {
      padding: 0.25rem 0.5rem !important;
    }
    .persona-widget-composer:not(.persona-pill-composer) {
      padding: 0.1rem 0 !important;
      gap: 0.25rem !important;
      align-items: center !important;
    }
    .persona-widget-body {
      padding-top: 0.25rem !important;
      padding-bottom: 0.25rem !important;
    }
    .persona-widget-messages {
      gap: 0.4rem !important;
      padding-top: 0.1rem !important;
      padding-bottom: 0.1rem !important;
    }
    /* One-line composer; Persona's JS maxes at 3 rows otherwise. */
    textarea[data-persona-composer-input],
    textarea.persona-composer-textarea,
    [data-persona-composer-input] {
      height: 1.35rem !important;
      min-height: 1.35rem !important;
      max-height: 1.35rem !important;
      line-height: 1.35rem !important;
      padding-top: 0 !important;
      padding-bottom: 0 !important;
      overflow-y: auto !important;
      resize: none !important;
    }
    /* Scroll-to-bottom FAB sits on the transcript; hide it in this dock. */
    .persona-scroll-to-bottom-indicator,
    [data-persona-scroll-to-bottom],
    button.persona-scroll-to-bottom-indicator {
      display: none !important;
    }
  `;

  function injectCompactChrome(root) {
    if (!root) return;
    const visit = (node) => {
      if (!node) return;
      if (node instanceof ShadowRoot) {
        let style = node.querySelector("style[data-tripmap-compact]");
        if (!style) {
          style = document.createElement("style");
          style.setAttribute("data-tripmap-compact", "");
          node.appendChild(style);
        }
        style.textContent = COMPACT_CSS;
        node.querySelectorAll("*").forEach((el) => {
          if (el.shadowRoot) visit(el.shadowRoot);
        });
        return;
      }
      if (node.shadowRoot) visit(node.shadowRoot);
      if (node.querySelectorAll) {
        node.querySelectorAll("*").forEach((el) => {
          if (el.shadowRoot) visit(el.shadowRoot);
        });
      }
    };
    visit(root);
  }

  async function mountPersona() {
    const host = document.getElementById("persona-dock-host");
    const detailChat = document.getElementById("detail-chat");
    const toggle = document.getElementById("btn-chat-toggle");
    if (!host || !detailChat) return;

    await loadPersonaStylesheet();
    const AW = await loadScript(PERSONA_JS);
    if (!AW?.initAgentWidget) {
      throw new Error("Persona initAgentWidget missing");
    }

    const apiUrl = new URL("api/chat", window.location.href).pathname;

    // Dock is left/right only; we size #detail-chat vertically and fill that
    // pane with a 100%-wide dock (empty content column + full chat panel).
    const controller = AW.initAgentWidget({
      target: host,
      useShadowDom: true,
      config: {
        apiUrl,
        suggestionChips: [],
        colorScheme: "light",
        // Hide the "Online" / connection chip under the composer.
        statusIndicator: { visible: false },
        features: {
          // Floating ↓ over the transcript steals space in the short dock.
          scrollToBottom: { enabled: false },
        },
        layout: {
          header: {
            showSubtitle: false,
          },
        },
        launcher: {
          enabled: true,
          mountMode: "docked",
          dock: {
            side: "right",
            width: "100%",
            animate: false,
            reveal: "resize",
            maxHeight: false,
          },
          title: "Trip assistant",
          subtitle: "Nudge this itinerary",
          agentIconName: "bot",
          headerIconName: "bot",
          agentIconSize: "28px",
          headerIconSize: "28px",
          closeButtonSize: "28px",
        },
        theme: {
          semantic: {
            colors: {
              accent: "#0f5c5c",
              primary: "#0f5c5c",
              surface: "#f3efe6",
              background: "#f3efe6",
              text: "#1a1f1c",
              textMuted: "#5c635c",
              border: "rgba(26, 31, 28, 0.12)",
            },
          },
          components: {
            panel: {
              borderRadius: "0",
            },
            // Header defaults to palette primary (near-black); force cream chrome.
            // Note: Persona also applies py-5/py-4 utility classes — see injectCompactChrome.
            header: {
              background: "#e6dfd0",
              border: "rgba(26, 31, 28, 0.12)",
              borderRadius: "0",
              iconBackground: "#0f5c5c",
              iconForeground: "#f3efe6",
              titleForeground: "#1a1f1c",
              subtitleForeground: "#5c635c",
              actionIconForeground: "#5c635c",
            },
            composer: {
              padding: "0.2rem 0.35rem",
              gap: "0.3rem",
              shadow: "none",
            },
            input: {
              padding: "0.35rem 0.55rem",
            },
            introCard: {
              padding: "0.5rem",
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

    const compactRoot = detailChat.parentElement || detailChat;
    injectCompactChrome(compactRoot);
    // Shadow roots may appear slightly after dock mount / first open.
    requestAnimationFrame(() => injectCompactChrome(compactRoot));
    setTimeout(() => injectCompactChrome(compactRoot), 200);

    let syncing = false;
    function setChatOpen(open, { fromWidget } = {}) {
      const next = Boolean(open);
      document.body.classList.toggle("chat-open", next);
      detailChat.setAttribute("aria-hidden", next ? "false" : "true");
      if (toggle) {
        toggle.setAttribute("aria-pressed", String(next));
        toggle.title = next ? "Close trip assistant" : "Trip assistant";
      }
      if (next) {
        requestAnimationFrame(() => injectCompactChrome(compactRoot));
      }
      if (fromWidget || syncing) return;
      syncing = true;
      try {
        if (next) controller.open();
        else controller.close();
      } finally {
        syncing = false;
      }
    }

    if (typeof controller?.on === "function") {
      controller.on("widget:opened", () => setChatOpen(true, { fromWidget: true }));
      controller.on("widget:closed", () => setChatOpen(false, { fromWidget: true }));
    }

    if (toggle) {
      toggle.hidden = false;
      toggle.addEventListener("click", () => {
        setChatOpen(!document.body.classList.contains("chat-open"));
      });
    }

    window.tripmap = window.tripmap || {};
    window.tripmap.setChatOpen = (open) => setChatOpen(open);
    window.tripmap.isChatOpen = () => document.body.classList.contains("chat-open");

    setChatOpen(false);
  }

  async function maybeEnableChat() {
    const toggle = document.getElementById("btn-chat-toggle");
    try {
      const res = await fetch("/auth/me", { credentials: "include" });
      if (!res.ok) return;
      const me = await res.json();
      if (!me.authenticated || !me.chat_enabled) return;
      await mountPersona();
      document.body.classList.add("chat-enabled");
    } catch (err) {
      console.warn("tripmap chat unavailable:", err);
      if (toggle) {
        toggle.hidden = false;
        toggle.disabled = true;
        toggle.title = `Chat failed to load: ${err?.message || err}`;
      }
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
