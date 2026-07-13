(() => {
  // Route lines (match KML semantics). Markers use separate fills so they
  // don't disappear into the path — ink for waypoints, red for lodging.
  const COLORS = {
    driveLine: "#2563eb",
    hikeLine: "#2f7d4a",
    ferryLine: "#c45e14",
    marker: "#1c1917",
    overnight: "#c62828",
    airport: "#4338ca",
  };

  const state = {
    trip: null,
    dayIndex: 0,
    mode: "list",
    showFullTrip: false,
    map: null,
    layers: L.layerGroup(),
    lastBounds: null,
    geoCache: new Map(),
  };

  const el = {
    title: document.getElementById("trip-title"),
    meta: document.getElementById("trip-meta"),
    dayIndex: document.getElementById("day-index"),
    detail: document.getElementById("detail"),
    map: document.getElementById("map"),
    offline: document.getElementById("offline-dot"),
    tileBanner: document.getElementById("tile-banner"),
    modeToggle: document.getElementById("mode-toggle"),
    btnDays: document.getElementById("btn-days"),
    btnFull: document.getElementById("btn-full-trip"),
    picker: document.getElementById("day-picker"),
    pickerList: document.getElementById("day-picker-list"),
    daySearch: document.getElementById("day-search"),
    closePicker: document.getElementById("btn-close-picker"),
    lightbox: document.getElementById("lightbox"),
    lightboxImg: document.getElementById("lightbox-img"),
    lightboxCap: document.getElementById("lightbox-caption"),
    lightboxClose: document.getElementById("lightbox-close"),
  };

  function notesKey(dayNum) {
    return `tripmap:${state.trip.id}:day:${dayNum}:notes`;
  }

  function kindLabel(kind) {
    if (kind === "hike") return "Hike";
    if (kind === "ferry") return "Ferry";
    if (kind === "rest") return "Rest";
    return "Drive";
  }

  function stopTypeLabel(type) {
    const labels = {
      depart: "Depart",
      overnight: "Overnight",
      attraction: "Attraction",
      viewpoint: "Viewpoint",
      trailhead: "Trailhead",
      hut: "Hut",
      ferry_terminal: "Ferry",
      airport: "Airport",
      via: "Via",
    };
    return labels[type] || type || "Stop";
  }

  function formatDriveStats(d) {
    const dist = d.drive_dist ?? d.drive_km; // drive_km: older bundles
    const unit = state.trip?.units === "mi" ? "mi" : "km";
    const parts = [];
    if (dist) {
      const n = Number.isInteger(dist) ? String(dist) : Number(dist).toFixed(1);
      parts.push(`${n} ${unit}`);
    }
    if (d.drive_min) {
      const h = Math.floor(d.drive_min / 60);
      const m = d.drive_min % 60;
      parts.push(h > 0 ? (m > 0 ? `${h}h ${m}m` : `${h}h`) : `${m} min`);
    }
    return parts.join(" · ");
  }

  function initMap() {
    state.map = L.map(el.map, { zoomControl: true, attributionControl: true });
    const tiles = L.tileLayer("https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png", {
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a> &copy; <a href="https://carto.com/">CARTO</a>',
      maxZoom: 19,
    });
    tiles.on("tileerror", () => {
      el.tileBanner.hidden = false;
    });
    tiles.addTo(state.map);
    state.layers.addTo(state.map);
    state.map.setView([52.1, 5.1], 7);
  }

  function renderDayIndex(container, filter = "") {
    const q = filter.trim().toLowerCase();
    container.innerHTML = "";
    state.trip.days.forEach((d, i) => {
      const hay = `${d.day} ${d.title} ${(d.stops || []).map((s) => s.name).join(" ")}`.toLowerCase();
      if (q && !hay.includes(q)) return;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = `day-row kind-${d.kind || "drive"}`;
      if (i === state.dayIndex) btn.classList.add("is-active");
      const stats = formatDriveStats(d);
      const sub = stats ? `${kindLabel(d.kind)} · ${stats}` : kindLabel(d.kind);
      btn.innerHTML = `
        <span class="day-row-num">Day ${String(d.day).padStart(2, "0")}</span>
        <span class="day-row-title" title="${escapeAttr(d.title)}">${escapeHtml(d.title)}</span>
        <span class="day-row-sub">${escapeHtml(sub)}</span>`;
      btn.addEventListener("click", () => selectDay(i, true));
      container.appendChild(btn);
    });
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function escapeAttr(s) {
    return escapeHtml(s).replace(/'/g, "&#39;");
  }

  function renderDetail(d) {
    const flags = [];
    if (d.hike) flags.push('<span class="flag hike">Hike</span>');
    if (d.ferry) flags.push('<span class="flag ferry">Ferry</span>');

    let photo = "";
    if (d.photo) {
      const caption = d.photo_caption || d.title;
      photo = `<button type="button" class="detail-photo" title="${escapeAttr(caption)}" data-photo="${escapeAttr(d.photo)}" data-caption="${escapeAttr(caption)}">
        <img src="${escapeAttr(d.photo)}" alt="${escapeAttr(caption)}" title="${escapeAttr(caption)}" loading="lazy" />
      </button>`;
    }

    const stops = (d.stops || [])
      .map((s) => {
        const stopCap = s.photo_caption || s.name;
        return `<li class="stop">
        <button type="button" data-lat="${s.lat}" data-lon="${s.lon}">
          <span class="stop-name">${escapeHtml(s.name)}</span>
          <span class="stop-type">${escapeHtml(stopTypeLabel(s.type))}</span>
        </button>
        ${
          s.photo
            ? `<img class="stop-thumb" src="${escapeAttr(s.photo)}" alt="${escapeAttr(stopCap)}" title="${escapeAttr(stopCap)}" loading="lazy" data-photo="${escapeAttr(s.photo)}" data-caption="${escapeAttr(stopCap)}" />`
            : ""
        }
      </li>`;
      })
      .join("");

    const saved = localStorage.getItem(notesKey(d.day)) || "";

    const driveStats = formatDriveStats(d);
    const i = state.dayIndex;
    const n = state.trip.days.length;
    const prevDisabled = i <= 0 ? "disabled" : "";
    const nextDisabled = i >= n - 1 ? "disabled" : "";
    const prevTitle = i > 0 ? escapeAttr(state.trip.days[i - 1].title) : "";
    const nextTitle = i < n - 1 ? escapeAttr(state.trip.days[i + 1].title) : "";

    el.detail.innerHTML = `
      <p class="detail-micro">Day ${d.day}</p>
      <h2>${escapeHtml(d.title)}</h2>
      <div>${flags.join("")}</div>
      ${driveStats ? `<p class="detail-stats">${escapeHtml(driveStats)}</p>` : ""}
      ${d.notes ? `<p class="detail-notes">${escapeHtml(d.notes)}</p>` : ""}
      ${photo}
      <ul class="stops">${stops || "<li class=\"stop\"><span class=\"stop-type\">No stops</span></li>"}</ul>
      <details class="notes-disclosure">
        <summary>Your notes</summary>
        <textarea id="local-notes" aria-label="Local notes for this day">${escapeHtml(saved)}</textarea>
      </details>
      <nav class="day-nav" aria-label="Adjacent days">
        <button type="button" class="day-nav-btn" data-dir="-1" ${prevDisabled} title="${prevTitle}">
          <span class="day-nav-dir" aria-hidden="true">‹</span> Prev
        </button>
        <span class="day-nav-pos">Day ${d.day}/${n}</span>
        <button type="button" class="day-nav-btn" data-dir="1" ${nextDisabled} title="${nextTitle}">
          Next <span class="day-nav-dir" aria-hidden="true">›</span>
        </button>
      </nav>`;

    el.detail.querySelectorAll(".day-nav-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const dir = Number(btn.dataset.dir);
        const next = state.dayIndex + dir;
        if (next < 0 || next >= state.trip.days.length) return;
        selectDay(next, true);
      });
    });

    el.detail.querySelectorAll("[data-photo]").forEach((node) => {
      node.addEventListener("click", (e) => {
        e.preventDefault();
        openLightbox(node.getAttribute("data-photo"), node.getAttribute("data-caption"));
      });
    });

    el.detail.querySelectorAll(".stop button[data-lat]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const lat = Number(btn.dataset.lat);
        const lon = Number(btn.dataset.lon);
        if (window.matchMedia("(max-width: 899px)").matches) {
          setMode("map", { lat, lon, zoom: Math.max(state.map.getZoom() || 0, 12) });
        } else {
          state.map.setView([lat, lon], Math.max(state.map.getZoom(), 12));
        }
      });
    });

    const ta = el.detail.querySelector("#local-notes");
    if (ta) {
      ta.addEventListener("input", () => {
        localStorage.setItem(notesKey(d.day), ta.value);
      });
    }
  }

  async function loadGeo(path) {
    if (state.geoCache.has(path)) return state.geoCache.get(path);
    const res = await fetch(path);
    if (!res.ok) throw new Error(`Failed to load ${path}`);
    const data = await res.json();
    state.geoCache.set(path, data);
    return data;
  }

  function styleFeature(feature) {
    if (feature.properties?.kind === "route") {
      const style = feature.properties.style || "driveLine";
      return {
        color: COLORS[style] || COLORS.driveLine,
        weight: 4,
        opacity: 0.9,
      };
    }
    return {};
  }

  function pointToLayer(feature, latlng) {
    const t = feature.properties?.type || "";
    let fill = COLORS.marker;
    let stroke = "#fff";
    if (t === "overnight") {
      fill = COLORS.overnight;
    } else if (t === "depart") {
      fill = COLORS.marker;
    } else if (t === "trailhead" || t === "hut") {
      stroke = COLORS.hikeLine;
    } else if (t === "ferry_terminal") {
      stroke = COLORS.ferryLine;
    } else if (t === "airport") {
      fill = COLORS.airport;
    }
    return L.circleMarker(latlng, {
      radius: 7,
      color: stroke,
      weight: t === "trailhead" || t === "hut" || t === "ferry_terminal" ? 2.5 : 1.5,
      fillColor: fill,
      fillOpacity: 0.95,
    }).bindPopup(feature.properties?.name || "");
  }

  async function renderMap() {
    state.layers.clearLayers();
    const bounds = [];

    const days = state.showFullTrip
      ? state.trip.days
      : [state.trip.days[state.dayIndex]];

    for (const d of days) {
      if (!d.geo) continue;
      try {
        const geo = await loadGeo(d.geo);
        const layer = L.geoJSON(geo, {
          style: styleFeature,
          pointToLayer,
        });
        layer.addTo(state.layers);
        const b = layer.getBounds();
        if (b.isValid()) bounds.push(b);
      } catch (err) {
        console.warn(err);
      }
    }

    state.lastBounds = null;
    if (bounds.length) {
      const merged = bounds[0];
      for (let i = 1; i < bounds.length; i++) merged.extend(bounds[i]);
      state.lastBounds = merged;
      // fitBounds while the pane is display:none computes a wrong zoom on mobile.
      if (mapPaneVisible()) {
        state.map.fitBounds(merged, { padding: [28, 28] });
      }
    }
    if (mapPaneVisible()) {
      state.map.invalidateSize({ animate: false });
    }
  }

  function mapPaneVisible() {
    return (
      !window.matchMedia("(max-width: 899px)").matches || state.mode === "map"
    );
  }

  function fitMapToContent() {
    if (!state.map) return;
    const b =
      state.lastBounds && state.lastBounds.isValid()
        ? state.lastBounds
        : state.layers.getLayers().length
          ? state.layers.getBounds()
          : null;
    if (b && b.isValid()) {
      state.map.fitBounds(b, { padding: [28, 28] });
    }
  }

  /** Leaflet needs a real size after the map pane is shown (mobile List→Map). */
  function whenMapLaidOut(fn) {
    const run = () => {
      state.map.invalidateSize({ animate: false });
      fn();
    };
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        run();
        setTimeout(run, 120);
      });
    });
  }

  async function selectDay(index, closePicker) {
    state.dayIndex = index;
    const d = state.trip.days[index];
    document.title = `${d.title} · ${state.trip.title}`;
    el.meta.textContent = `Day ${d.day} of ${state.trip.days.length}`;
    renderDayIndex(el.dayIndex);
    renderDayIndex(el.pickerList, el.daySearch.value);
    renderDetail(d);
    if (!state.showFullTrip) await renderMap();
    if (closePicker) el.picker.hidden = true;
    el.btnDays.textContent = "Days";
  }

  function setMode(mode, opts = {}) {
    state.mode = mode;
    document.body.classList.toggle("mode-list", mode === "list");
    document.body.classList.toggle("mode-map", mode === "map");
    el.modeToggle.querySelectorAll(".seg-btn").forEach((b) => {
      b.classList.toggle("is-active", b.dataset.mode === mode);
    });
    if (mode === "map") {
      whenMapLaidOut(() => {
        if (opts.lat != null && opts.lon != null) {
          state.map.setView(
            [opts.lat, opts.lon],
            opts.zoom ?? Math.max(state.map.getZoom() || 0, 12)
          );
        } else {
          fitMapToContent();
        }
      });
    }
  }

  function openLightbox(src, caption) {
    el.lightboxImg.src = src;
    el.lightboxImg.alt = caption || "";
    el.lightboxCap.textContent = caption || "";
    el.lightbox.hidden = false;
  }

  function updateOnline() {
    const online = navigator.onLine;
    el.offline.hidden = false;
    el.offline.classList.toggle("is-online", online);
    el.offline.title = online ? "Online" : "Offline";
    if (online) el.tileBanner.hidden = true;
  }

  async function boot() {
    initMap();
    setMode(window.matchMedia("(max-width: 899px)").matches ? "list" : "list");
    document.body.classList.add("mode-list");

    const res = await fetch("trip.json");
    if (!res.ok) {
      el.title.textContent = "Couldn’t load trip";
      el.meta.textContent = "Retry after checking that trip.json is next to index.html.";
      return;
    }
    state.trip = await res.json();
    el.title.textContent = state.trip.title;
    document.title = state.trip.title;

    el.modeToggle.hidden = false;
    el.btnDays.hidden = false;

    await selectDay(0, false);
    updateOnline();
  }

  el.modeToggle.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-mode]");
    if (btn) setMode(btn.dataset.mode);
  });

  el.btnDays.addEventListener("click", () => {
    el.picker.hidden = false;
    el.daySearch.value = "";
    renderDayIndex(el.pickerList);
    el.daySearch.focus();
  });
  el.closePicker.addEventListener("click", () => {
    el.picker.hidden = true;
  });
  el.daySearch.addEventListener("input", () => {
    renderDayIndex(el.pickerList, el.daySearch.value);
  });

  el.btnFull.addEventListener("click", async () => {
    state.showFullTrip = !state.showFullTrip;
    el.btnFull.setAttribute("aria-pressed", String(state.showFullTrip));
    el.btnFull.textContent = state.showFullTrip ? "This day" : "Full trip";
    await renderMap();
  });

  el.lightboxClose.addEventListener("click", () => {
    el.lightbox.hidden = true;
  });
  el.lightbox.addEventListener("click", (e) => {
    if (e.target === el.lightbox) el.lightbox.hidden = true;
  });

  window.addEventListener("online", updateOnline);
  window.addEventListener("offline", updateOnline);

  window.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      if (!el.lightbox.hidden) {
        el.lightbox.hidden = true;
        return;
      }
      if (!el.picker.hidden) {
        el.picker.hidden = true;
        return;
      }
    }
    if (!state.trip) return;
    if (e.target.matches("textarea, input")) return;
    if (e.key === "j") selectDay(Math.min(state.dayIndex + 1, state.trip.days.length - 1), true);
    if (e.key === "k") selectDay(Math.max(state.dayIndex - 1, 0), true);
  });

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("./sw.js").catch(() => {});
  }

  boot().catch((err) => {
    console.error(err);
    el.title.textContent = "Error loading trip";
    el.meta.textContent = String(err.message || err);
  });
})();
