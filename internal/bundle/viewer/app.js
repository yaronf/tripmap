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
    flight: "#4338ca",
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
    sharedNotes: { days: {} },
    notesSaveTimer: null,
    notesDirty: false,
  };

  const el = {
    title: document.getElementById("trip-title"),
    meta: document.getElementById("trip-meta"),
    app: document.getElementById("app"),
    dayIndex: document.getElementById("day-index"),
    detail: document.getElementById("detail"),
    detailBody: document.getElementById("detail-body"),
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

  function dayNote(dayNum) {
    return state.sharedNotes.days?.[String(dayNum)] || "";
  }

  function setDayNote(dayNum, text) {
    if (!state.sharedNotes.days) state.sharedNotes.days = {};
    state.sharedNotes.days[String(dayNum)] = text;
  }

  async function loadSharedNotes() {
    try {
      const res = await fetch("api/notes");
      if (!res.ok) return;
      const doc = await res.json();
      state.sharedNotes = {
        days: doc.days && typeof doc.days === "object" ? doc.days : {},
        updated_at: doc.updated_at,
      };
    } catch {
      // Offline: SW may still satisfy fetch from cache; if not, keep empty.
    }
  }

  function scheduleSaveNotes(dayNum) {
    state.notesDirty = true;
    if (state.notesSaveTimer) clearTimeout(state.notesSaveTimer);
    state.notesSaveTimer = setTimeout(() => {
      saveSharedNotes(dayNum);
    }, 500);
  }

  async function saveSharedNotes() {
    if (!navigator.onLine) return;
    try {
      const res = await fetch("api/notes", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ days: state.sharedNotes.days || {} }),
      });
      if (res.ok) {
        const doc = await res.json();
        state.sharedNotes.updated_at = doc.updated_at;
        state.notesDirty = false;
      }
    } catch {
      // Leave dirty; user can edit offline but writes do not queue.
    }
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
      flight: "Flight",
      via: "Via",
    };
    return labels[type] || type || "Stop";
  }

  function isGoogleMapsURL(url) {
    try {
      const u = new URL(url);
      const host = u.hostname.toLowerCase();
      if (host === "g.page" || host === "maps.app.goo.gl" || host === "goo.gl") return true;
      if (host === "maps.google.com" || host.endsWith(".google.com") || host.endsWith(".google.co.nz")) {
        return /maps/i.test(host + u.pathname + u.search);
      }
      return false;
    } catch {
      return false;
    }
  }

  function mapsPinURL(stop) {
    const override = typeof stop?.maps_url === "string" ? stop.maps_url.trim() : "";
    // Chat sometimes stores a venue website in maps_url — only trust Google Maps links.
    if (override && isGoogleMapsURL(override)) return override;
    return mapsSearchURL(Number(stop?.lat), Number(stop?.lon));
  }

  function mapsSearchURL(lat, lon) {
    if (!Number.isFinite(lat) || !Number.isFinite(lon)) return "";
    return `https://www.google.com/maps/search/?api=1&query=${lat},${lon}`;
  }

  function mapsDirectionsURL(points) {
    const pts = (points || []).filter(
      (p) => Number.isFinite(p.lat) && Number.isFinite(p.lon)
    );
    if (pts.length < 2) return "";
    const origin = `${pts[0].lat},${pts[0].lon}`;
    const destination = `${pts[pts.length - 1].lat},${pts[pts.length - 1].lon}`;
    let url = `https://www.google.com/maps/dir/?api=1&origin=${origin}&destination=${destination}&travelmode=driving`;
    if (pts.length > 2) {
      const waypoints = pts
        .slice(1, -1)
        .slice(0, 8) // Maps URL practical limit
        .map((p) => `${p.lat},${p.lon}`)
        .join("|");
      if (waypoints) url += `&waypoints=${encodeURIComponent(waypoints)}`;
    }
    return url;
  }

  function formatStopInfo(info) {
    if (!info) return "";
    const parts = [];
    if (info.links && info.links.length) {
      const links = info.links
        .map((l) => {
          const label = l.title || l.type || "Link";
          return `<a class="stop-info-link" href="${escapeAttr(l.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(label)}</a>`;
        })
        .join(" · ");
      parts.push(`<div class="stop-info-row">${links}</div>`);
    }
    if (info.stats) {
      const s = info.stats;
      const bits = [];
      if (s.distance_km != null) bits.push(`${s.distance_km} km`);
      if (s.duration) bits.push(s.duration);
      if (s.ascent_m != null) bits.push(`↑ ${s.ascent_m} m`);
      if (s.difficulty) bits.push(s.difficulty);
      if (bits.length) parts.push(`<div class="stop-info-row stop-info-stats">${escapeHtml(bits.join(" · "))}</div>`);
    }
    if (info.logistics) {
      const L = info.logistics;
      if (L.opening_hours) {
        parts.push(`<div class="stop-info-row"><span class="stop-info-label">Hours</span> ${escapeHtml(L.opening_hours)}</div>`);
      }
      if (L.parking) {
        parts.push(`<div class="stop-info-row"><span class="stop-info-label">Parking</span> ${escapeHtml(L.parking)}</div>`);
      }
    }
    if (info.warnings && info.warnings.length) {
      parts.push(
        `<ul class="stop-info-list stop-info-warnings">${info.warnings
          .map((w) => `<li>${escapeHtml(w)}</li>`)
          .join("")}</ul>`
      );
    }
    if (info.highlights && info.highlights.length) {
      parts.push(
        `<ul class="stop-info-list stop-info-highlights">${info.highlights
          .map((h) => `<li>${escapeHtml(h)}</li>`)
          .join("")}</ul>`
      );
    }
    if (!parts.length) return "";
    return `<div class="stop-info">${parts.join("")}</div>`;
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
    L.control.scale({ metric: true, imperial: false, maxWidth: 200 }).addTo(state.map);
    state.layers.addTo(state.map);
    state.map.setView([52.1, 5.1], 7);
  }

  function formatDayDate(iso) {
    if (!iso) return "";
    const d = new Date(`${iso}T12:00:00`);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleDateString(undefined, {
      weekday: "short",
      day: "numeric",
      month: "short",
    });
  }

  function dayNumLabel(d) {
    const n = `Day ${d.day}`;
    const date = formatDayDate(d.date);
    return date ? `${n} · ${date}` : n;
  }

  function renderDayIndex(container, filter = "") {
    const q = filter.trim().toLowerCase();
    container.innerHTML = "";
    let activeBtn = null;
    state.trip.days.forEach((d, i) => {
      const dateLabel = formatDayDate(d.date);
      const hay = `${d.day} ${d.date || ""} ${dateLabel} ${d.title} ${(d.stops || [])
        .map((s) => s.name)
        .join(" ")}`.toLowerCase();
      if (q && !hay.includes(q)) return;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = `day-row kind-${d.kind || "drive"}`;
      if (i === state.dayIndex) {
        btn.classList.add("is-active");
        activeBtn = btn;
      }
      const stats = formatDriveStats(d);
      const sub = stats ? `${kindLabel(d.kind)} · ${stats}` : kindLabel(d.kind);
      btn.innerHTML = `
        <span class="day-row-num">${escapeHtml(dayNumLabel(d))}</span>
        <span class="day-row-title" title="${escapeAttr(d.title)}">${escapeHtml(d.title)}</span>
        <span class="day-row-sub">${escapeHtml(sub)}</span>`;
      btn.addEventListener("click", () => selectDay(i, true));
      container.appendChild(btn);
    });
    // Keep the active day visible in the scrollable day list / picker.
    if (activeBtn) {
      requestAnimationFrame(() => {
        activeBtn.scrollIntoView({ block: "nearest", inline: "nearest" });
      });
    }
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
        const infoHtml = formatStopInfo(s.info);
        const stopNotes = s.notes
          ? `<p class="stop-notes">${escapeHtml(s.notes)}</p>`
          : "";
        const mapsHref = mapsPinURL(s);
        const mapsLink = mapsHref
          ? `<a class="maps-link" href="${escapeAttr(mapsHref)}" target="_blank" rel="noopener noreferrer" aria-label="Open ${escapeAttr(s.name || "stop")} in Google Maps" title="Google Maps"><img class="maps-pin" src="maps-pin.png" alt="" width="22" height="22" /></a>`
          : "";
        return `<li class="stop">
        <div class="stop-row">
          <button type="button" data-lat="${s.lat}" data-lon="${s.lon}">
            <span class="stop-name">${escapeHtml(s.name)}</span>
            <span class="stop-type">${escapeHtml(stopTypeLabel(s.type))}</span>
          </button>
          ${mapsLink}
        </div>
        ${stopNotes}
        ${
          s.photo
            ? `<img class="stop-thumb" src="${escapeAttr(s.photo)}" alt="${escapeAttr(stopCap)}" title="${escapeAttr(stopCap)}" loading="lazy" data-photo="${escapeAttr(s.photo)}" data-caption="${escapeAttr(stopCap)}" />`
            : ""
        }
        ${infoHtml}
      </li>`;
      })
      .join("");

    const saved = dayNote(d.day);
    const dayMapsHref = mapsDirectionsURL(d.stops || []);
    const dayMaps = dayMapsHref
      ? `<p class="day-maps"><a class="maps-link maps-link-day" href="${escapeAttr(dayMapsHref)}" target="_blank" rel="noopener noreferrer" aria-label="Directions in Google Maps" title="Directions in Google Maps"><img class="maps-pin" src="maps-pin.png" alt="" width="22" height="22" /><span>Directions</span></a></p>`
      : "";

    const driveStats = formatDriveStats(d);
    const n = state.trip.days.length;
    const dateLabel = formatDayDate(d.date);
    const micro = dateLabel ? `Day ${d.day} · ${dateLabel}` : `Day ${d.day}`;

    const body = el.detailBody || el.detail;
    body.innerHTML = `
      <p class="detail-micro">${escapeHtml(micro)}</p>
      <h2>${escapeHtml(d.title)}</h2>
      <div>${flags.join("")}</div>
      ${driveStats ? `<p class="detail-stats">${escapeHtml(driveStats)}</p>` : ""}
      ${dayMaps}
      ${
        d.notes
          ? `<section class="day-notes" aria-label="Day notes">
        <h3 class="day-notes-heading">Notes</h3>
        <p class="detail-notes">${escapeHtml(d.notes)}</p>
      </section>`
          : ""
      }
      ${photo}
      <ul class="stops">${stops || "<li class=\"stop\"><span class=\"stop-type\">No stops</span></li>"}</ul>
      <section class="shared-notes" aria-label="Comments">
        <div class="shared-notes-header">
          <h3 class="shared-notes-heading">Comments</h3>
          <button type="button" id="shared-notes-edit" class="icon-btn" aria-label="${
            saved ? "Edit comments" : "Add comments"
          }" title="${saved ? "Edit" : "Add"}">
            <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>
            </svg>
          </button>
        </div>
        <p id="shared-notes-display" class="shared-notes-body${saved ? "" : " is-empty"}">${
          saved ? escapeHtml(saved) : "No comments yet."
        }</p>
        <div id="shared-notes-editor" class="shared-notes-editor" hidden>
          <textarea id="shared-notes" aria-label="Edit comments for this day">${escapeHtml(saved)}</textarea>
          ${!navigator.onLine ? `<p class="notes-offline-hint">Offline — edits won’t sync until you’re online.</p>` : ""}
          <button type="button" id="shared-notes-done" class="btn shared-notes-done">Done</button>
        </div>
      </section>`;

    body.querySelectorAll("[data-photo]").forEach((node) => {
      node.addEventListener("click", (e) => {
        e.preventDefault();
        openLightbox(node.getAttribute("data-photo"), node.getAttribute("data-caption"));
      });
    });

    body.querySelectorAll(".stop button[data-lat]").forEach((btn) => {
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

    const ta = body.querySelector("#shared-notes");
    const display = body.querySelector("#shared-notes-display");
    const editor = body.querySelector("#shared-notes-editor");
    const editBtn = body.querySelector("#shared-notes-edit");
    const doneBtn = body.querySelector("#shared-notes-done");
    const syncDisplay = (value) => {
      if (!display) return;
      const text = (value || "").trim();
      display.textContent = text || "No comments yet.";
      display.classList.toggle("is-empty", !text);
      if (editBtn) {
        const label = text ? "Edit comments" : "Add comments";
        editBtn.setAttribute("aria-label", label);
        editBtn.title = text ? "Edit" : "Add";
      }
    };
    const setEditing = (on) => {
      if (display) display.hidden = on;
      if (editor) editor.hidden = !on;
      if (editBtn) editBtn.hidden = on;
      if (on && ta) {
        ta.focus();
        const len = ta.value.length;
        ta.setSelectionRange(len, len);
      }
    };
    if (editBtn) {
      editBtn.addEventListener("click", () => setEditing(true));
    }
    if (doneBtn) {
      doneBtn.addEventListener("click", () => {
        if (state.notesSaveTimer) {
          clearTimeout(state.notesSaveTimer);
          state.notesSaveTimer = null;
        }
        saveSharedNotes();
        setEditing(false);
      });
    }
    if (ta) {
      ta.addEventListener("input", () => {
        setDayNote(d.day, ta.value);
        syncDisplay(ta.value);
        scheduleSaveNotes(d.day);
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
    } else if (t === "airport" || t === "flight") {
      fill = COLORS.flight || COLORS.airport;
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
    const dateLabel = formatDayDate(d.date);
    const pos = `Day ${d.day}/${state.trip.days.length}`;
    // Mobile chrome is tight: Day N/M only. Desktop can include the date.
    el.meta.textContent = !isMobile() && dateLabel ? `${pos} · ${dateLabel}` : pos;
    renderDayIndex(el.dayIndex);
    renderDayIndex(el.pickerList, el.daySearch.value);
    renderDetail(d);
    if (!state.showFullTrip) await renderMap();
    if (closePicker) el.picker.hidden = true;
    el.btnDays.textContent = "Days";
  }

  function stepDay(dir) {
    if (!state.trip) return;
    const next = state.dayIndex + dir;
    if (next < 0 || next >= state.trip.days.length) return;
    selectDay(next, true);
  }

  function wait(ms) {
    return new Promise((r) => setTimeout(r, ms));
  }

  function reduceMotion() {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function isMobile() {
    return window.matchMedia("(max-width: 899px)").matches;
  }

  /**
   * Book swipe on mobile: full page in List mode; detail sheet only in Map
   * mode (Leaflet keeps the map). Returns the element to translate, or null.
   */
  function swipeSurface(target) {
    if (!isMobile() || !state.trip) return null;
    if (!el.lightbox.hidden || !el.picker.hidden) return null;
    if (target.closest("textarea, input, button, a, .shared-notes-editor, .chrome")) {
      return null;
    }
    if (state.mode === "list") {
      return el.app;
    }
    if (state.mode === "map") {
      if (target.closest(".map-pane, #map, .map-toolbar")) return null;
      if (target.closest("#detail")) return el.detail;
    }
    return null;
  }

  /** Book-style horizontal day turn (list: whole page; map: detail text). */
  function bindDaySwipe() {
    let surface = null;
    let x0 = 0;
    let y0 = 0;
    let rawDx = 0;
    let axis = null; // null | "h" | "v"
    let tracking = false;
    let turning = false;

    function setOffset(node, x, withTransition) {
      if (!node) return;
      node.style.transition = withTransition ? "transform 0.22s ease-out" : "none";
      node.style.transform = x ? `translate3d(${x}px,0,0)` : "";
    }

    function clearOffset(node) {
      if (!node) return;
      node.style.transition = "";
      node.style.transform = "";
    }

    function dampen(raw) {
      const atStart = state.dayIndex <= 0 && raw > 0;
      const atEnd = state.dayIndex >= state.trip.days.length - 1 && raw < 0;
      return atStart || atEnd ? raw * 0.28 : raw;
    }

    async function turnPage(dir, node) {
      turning = true;
      document.body.classList.remove("is-day-swiping");
      if (reduceMotion()) {
        setOffset(node, 0, false);
        clearOffset(node);
        await selectDay(state.dayIndex + dir, true);
        turning = false;
        surface = null;
        return;
      }
      const w = window.innerWidth;
      const outX = dir > 0 ? -w : w;
      const inX = dir > 0 ? w : -w;
      setOffset(node, outX, true);
      await wait(200);
      node.style.transition = "none";
      node.style.transform = `translate3d(${inX}px,0,0)`;
      await selectDay(state.dayIndex + dir, true);
      // After selectDay, list mode still uses #app; map mode reuses #detail.
      const slide = state.mode === "map" ? el.detail : el.app;
      if (slide !== node) clearOffset(node);
      void slide.offsetWidth;
      setOffset(slide, 0, true);
      await wait(220);
      clearOffset(slide);
      turning = false;
      surface = null;
    }

    function endGesture(commitDir) {
      tracking = false;
      axis = null;
      document.body.classList.remove("is-day-swiping");
      const node = surface;
      const next = state.dayIndex + commitDir;
      if (
        commitDir &&
        state.trip &&
        next >= 0 &&
        next < state.trip.days.length
      ) {
        turnPage(commitDir, node);
        return;
      }
      setOffset(node, 0, !reduceMotion());
      if (!reduceMotion()) {
        wait(220).then(() => {
          clearOffset(node);
          surface = null;
        });
      } else {
        clearOffset(node);
        surface = null;
      }
    }

    // Capture on document so map-mode detail sheet receives gestures;
    // list mode still uses #app as the sliding surface.
    document.addEventListener(
      "touchstart",
      (e) => {
        if (e.touches.length !== 1 || turning) return;
        const node = swipeSurface(e.target);
        if (!node) return;
        tracking = true;
        surface = node;
        axis = null;
        rawDx = 0;
        x0 = e.touches[0].clientX;
        y0 = e.touches[0].clientY;
      },
      { passive: true }
    );

    document.addEventListener(
      "touchmove",
      (e) => {
        if (!tracking || turning || !surface || e.touches.length !== 1) return;
        const x = e.touches[0].clientX;
        const y = e.touches[0].clientY;
        const rawX = x - x0;
        const rawY = y - y0;
        if (!axis) {
          if (Math.abs(rawX) < 10 && Math.abs(rawY) < 10) return;
          axis = Math.abs(rawX) > Math.abs(rawY) * 1.15 ? "h" : "v";
          if (axis === "v") {
            tracking = false;
            surface = null;
            return;
          }
          document.body.classList.add("is-day-swiping");
        }
        if (axis !== "h") return;
        e.preventDefault();
        rawDx = rawX;
        setOffset(surface, dampen(rawX), false);
      },
      { passive: false }
    );

    document.addEventListener(
      "touchcancel",
      () => {
        if (!tracking) return;
        endGesture(0);
      },
      { passive: true }
    );

    document.addEventListener(
      "touchend",
      () => {
        if (!tracking || axis !== "h") {
          tracking = false;
          axis = null;
          surface = null;
          return;
        }
        const w = window.innerWidth;
        const threshold = Math.min(88, w * 0.22);
        // Swipe left → next; swipe right → previous.
        let dir = 0;
        if (rawDx <= -threshold) dir = 1;
        else if (rawDx >= threshold) dir = -1;
        endGesture(dir);
      },
      { passive: true }
    );
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

  function todayISO() {
    const d = new Date();
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }

  /** Prefer ?day=N (YAML day number), else today's date when dated, else day 1. */
  function initialDayIndex() {
    const params = new URLSearchParams(location.search);
    if (params.has("day")) {
      const n = Number(params.get("day"));
      const byNum = state.trip.days.findIndex((d) => d.day === n);
      if (byNum >= 0) return byNum;
    }

    const days = state.trip.days;
    const dated = days.map((d, i) => (d.date ? { i, date: d.date } : null)).filter(Boolean);
    if (!dated.length) return 0;

    const today = todayISO();
    const exact = dated.find((d) => d.date === today);
    if (exact) return exact.i;
    // Outside the trip window → day 1
    if (today < dated[0].date || today > dated[dated.length - 1].date) {
      return 0;
    }

    let best = dated[0].i;
    for (const d of dated) {
      if (d.date <= today) best = d.i;
    }
    return best;
  }

  async function applyTrip(trip, { preserveDay } = {}) {
    const prevDayNum = preserveDay && state.trip?.days?.[state.dayIndex]?.day;
    state.trip = trip;
    state.geoCache.clear();
    await loadSharedNotes();
    el.title.textContent = state.trip.title;
    const pageTitle = /itinerary/i.test(state.trip.title)
      ? state.trip.title
      : `${state.trip.title} Itinerary`;
    document.title = pageTitle;

    el.modeToggle.hidden = false;
    el.btnDays.hidden = false;

    let idx = initialDayIndex();
    if (prevDayNum != null) {
      const found = state.trip.days.findIndex((d) => d.day === prevDayNum);
      if (found >= 0) idx = found;
    }
    await selectDay(idx, false);
    updateOnline();
  }

  async function reloadTrip() {
    const res = await fetch("trip.json", { cache: "no-store" });
    if (!res.ok) throw new Error("Could not reload trip.json");
    await applyTrip(await res.json(), { preserveDay: true });
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
    await applyTrip(await res.json());
  }

  window.tripmap = {
    reloadTrip,
    getTrip: () => state.trip,
    getDayIndex: () => state.dayIndex,
    getDayNumber: () => {
      const d = state.trip?.days?.[state.dayIndex];
      if (!d) return 0;
      const n = Number(d.day);
      return Number.isFinite(n) && n > 0 ? n : state.dayIndex + 1;
    },
  };

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

  function eventFromEditable(e) {
    const path = typeof e.composedPath === "function" ? e.composedPath() : [];
    for (const node of path) {
      if (!node || node.nodeType !== 1) continue;
      const tag = node.tagName;
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT") return true;
      if (node.isContentEditable) return true;
      const role = node.getAttribute?.("role");
      if (role === "textbox" || role === "searchbox" || role === "combobox") return true;
    }
    const t = e.target;
    if (t instanceof Element) {
      if (t.matches("textarea, input, select, [contenteditable=''], [contenteditable=true], [role=textbox]")) {
        return true;
      }
    }
    return false;
  }

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
    if (eventFromEditable(e)) return;
    if (e.key === "ArrowDown" || e.key === "j") {
      e.preventDefault();
      stepDay(1);
    }
    if (e.key === "ArrowUp" || e.key === "k") {
      e.preventDefault();
      stepDay(-1);
    }
  });

  bindDaySwipe();

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("./sw.js").catch(() => {});
  }

  boot().catch((err) => {
    console.error(err);
    el.title.textContent = "Error loading trip";
    el.meta.textContent = String(err.message || err);
  });
})();
