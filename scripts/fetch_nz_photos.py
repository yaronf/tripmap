#!/usr/bin/env python3
"""Fetch freely licensed New Zealand thumbs from Wikimedia Commons.

Rejects ultra-wide / ultra-tall source images so day photos work in the PWA
detail column (target landscape-ish ~4:3–16:9).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import time
import urllib.parse
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DIR = os.path.join(ROOT, "itineraries", "photos")
UA = {
    "User-Agent": "tripmap/1.0 (https://github.com/yaronf/tripmap; itinerary photos)"
}

# Soft window for detail-pane photos. Panoramas (>~1.9) look like thin strips.
MIN_ASPECT = 0.75  # allow mild portrait / square
MAX_ASPECT = 1.85  # reject ultra-wide panoramas

QUERIES = [
    ("auckland.jpg", "Auckland skyline harbour", "days 1–2"),
    ("tongariro.jpg", "Tongariro Emerald Lakes", "day 5"),
    ("wellington.jpg", "Wellington New Zealand waterfront boats", "day 6"),
    ("abel-tasman.jpg", "Abel Tasman National Park beach", "day 8"),
    ("punakaiki.jpg", "Punakaiki Pancake Rocks", "days 9–10"),
    ("franz-josef.jpg", "Franz Josef Glacier", "day 11"),
    ("wanaka.jpg", "Lake Wanaka mountains", "day 13"),
    ("milford.jpg", "Milford Sound Fiordland", "day 20"),
    ("queenstown.jpg", "Queenstown Lake Wakatipu", "days 22–23"),
    ("aoraki.jpg", "Aoraki Mount Cook Hooker Valley", "day 25"),
    ("tekapo.jpg", "Lake Tekapo Church of the Good Shepherd", "days 26–27"),
    ("christchurch.jpg", "Christchurch Cathedral Square", "day 28"),
]

# Extra search fallbacks when the primary query only hits panoramas.
FALLBACKS = {
    "wellington.jpg": [
        "Cable Car Wellington",
        "Te Papa Wellington exterior",
        "Oriental Bay Wellington",
        "Mount Victoria Wellington lookout",
    ],
}


def api(params: dict) -> dict:
    q = urllib.parse.urlencode({**params, "format": "json"})
    req = urllib.request.Request(
        "https://commons.wikimedia.org/w/api.php?" + q, headers=UA
    )
    with urllib.request.urlopen(req, timeout=90) as r:
        return json.load(r)


def strip_html(s: str) -> str:
    s = re.sub(r"<[^>]+>", "", s)
    return re.sub(r"\s+", " ", s).strip()


def image_info(title: str) -> dict | None:
    data = api(
        {
            "action": "query",
            "titles": title,
            "prop": "imageinfo",
            "iiprop": "url|user|extmetadata|size|mime|dimensions",
            "iiurlwidth": "1280",
        }
    )
    page = next(iter(data["query"]["pages"].values()))
    if "missing" in page or "imageinfo" not in page:
        return None
    return page["imageinfo"][0]


def aspect_ok(width: int, height: int) -> bool:
    if width <= 0 or height <= 0:
        return False
    r = width / height
    return MIN_ASPECT <= r <= MAX_ASPECT


def search_titles(query: str, limit: int = 20) -> list[str]:
    data = api(
        {
            "action": "query",
            "list": "search",
            "srsearch": f"filetype:bitmap {query}",
            "srnamespace": "6",
            "srlimit": str(limit),
        }
    )
    out = []
    for hit in data.get("query", {}).get("search", []):
        title = hit["title"]
        if title.lower().endswith((".jpg", ".jpeg", ".png")):
            out.append(title)
    return out


def pick_title(query: str, extra: list[str] | None = None) -> tuple[str, dict] | None:
    """Return (title, imageinfo) for the first search hit with a good aspect ratio."""
    queries = [query] + (extra or [])
    for q in queries:
        if q != query:
            print(f"  fallback search: {q}")
        for title in search_titles(q):
            time.sleep(0.8)
            info = image_info(title)
            if not info:
                continue
            w, h = info.get("width", 0), info.get("height", 0)
            r = (w / h) if h else 0
            if not aspect_ok(w, h):
                print(f"  skip {title} ({w}x{h} = {r:.2f})")
                continue
            print(f"  accept {title} ({w}x{h} = {r:.2f})")
            return title, info
    return None


def download(info: dict, dest: str) -> None:
    thumb = info.get("thumburl") or info["url"]
    time.sleep(1.0)
    req = urllib.request.Request(thumb, headers=UA)
    with urllib.request.urlopen(req, timeout=120) as r:
        open(dest, "wb").write(r.read())


def jpeg_size(path: str) -> tuple[int, int] | None:
    """Return (w,h) for a JPEG without Pillow."""
    with open(path, "rb") as f:
        if f.read(2) != b"\xff\xd8":
            return None
        while True:
            b = f.read(1)
            while b and b != b"\xff":
                b = f.read(1)
            while b == b"\xff":
                b = f.read(1)
            if not b:
                return None
            if b in (b"\xc0", b"\xc1", b"\xc2"):
                f.read(3)
                h, w = int.from_bytes(f.read(2), "big"), int.from_bytes(f.read(2), "big")
                return w, h
            seglen = int.from_bytes(f.read(2), "big")
            f.read(seglen - 2)


def local_aspect_ok(path: str) -> bool:
    wh = jpeg_size(path)
    if not wh:
        # PNG etc. — probe via commons was already ok if we just wrote it
        return True
    return aspect_ok(*wh)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--replace",
        nargs="*",
        help="Only replace these filenames (e.g. wellington.jpg). Default: fill missing.",
    )
    ap.add_argument(
        "--all-bad",
        action="store_true",
        help=f"Replace any on-disk JPEG whose aspect is outside {MIN_ASPECT}–{MAX_ASPECT}",
    )
    args = ap.parse_args()
    os.makedirs(DIR, exist_ok=True)

    replace = set(args.replace or [])
    if args.all_bad:
        for name in os.listdir(DIR):
            if not name.lower().endswith((".jpg", ".jpeg")):
                continue
            path = os.path.join(DIR, name)
            if not local_aspect_ok(path):
                wh = jpeg_size(path)
                print(f"flag wide/tall: {name} {wh}")
                replace.add(name)

    credits: list[tuple[str, str, str, str, str, str]] = []
    for local, query, hint in QUERIES:
        dest = os.path.join(DIR, local)
        exists = os.path.exists(dest) and os.path.getsize(dest) > 20_000
        if exists and local not in replace:
            if not local_aspect_ok(dest) and not args.all_bad and not replace:
                print(f"warn: {local} aspect out of range (use --all-bad)")
            print(f"skip existing {local}")
            continue

        print(f"search: {query}")
        try:
            picked = pick_title(query, FALLBACKS.get(local))
        except Exception as e:
            print(f"  search fail: {e}")
            time.sleep(5)
            continue
        time.sleep(1.0)
        if not picked:
            print(f"  no aspect-ok hit for {query}")
            continue
        title, info = picked
        meta = info.get("extmetadata", {})
        artist = strip_html(meta.get("Artist", {}).get("value", info.get("user", "?")))
        license_short = meta.get("LicenseShortName", {}).get("value", "?")
        try:
            download(info, dest)
        except Exception as e:
            print(f"  FAIL {e}")
            time.sleep(5)
            continue
        print(f"  wrote {local} ({os.path.getsize(dest) // 1024} KiB) {license_short}")
        credits.append(
            (
                local,
                title.replace("File:", ""),
                artist,
                license_short,
                hint,
                "https://commons.wikimedia.org/wiki/" + urllib.parse.quote(title),
            )
        )
        time.sleep(1.5)

    # Merge into CREDITS.md: keep old lines for files we didn't touch, update replaced.
    path = os.path.join(DIR, "CREDITS.md")
    by_file: dict[str, str] = {}
    if os.path.exists(path):
        for line in open(path):
            m = re.match(r"- `([^`]+)` .*", line)
            if m:
                by_file[m.group(1)] = line.rstrip("\n")
    for local, orig, artist, lic, hint, url in credits:
        by_file[local] = f"- `{local}` ({hint}) — {orig}; {artist}; {lic}; {url}"

    lines = [
        "# Photo credits\n\n",
        "Images from [Wikimedia Commons](https://commons.wikimedia.org/), "
        "resized (~1280px) for the tripmap PWA.\n\n",
        f"Aspect filter when fetching: {MIN_ASPECT:.2f}–{MAX_ASPECT:.2f} (width/height).\n\n",
    ]
    for local, _, hint in QUERIES:
        if local in by_file:
            lines.append(by_file[local] + "\n")
    open(path, "w").write("".join(lines))
    print("Wrote", path)


if __name__ == "__main__":
    main()
