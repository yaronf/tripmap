
# tripmap (MVP)

Usage:

    go run . --input itinerary.yaml --output trip.kml
    go run . --input itinerary.yaml --output trip.kml --route osrm

The MVP:
- Reads a YAML itinerary.
- Emits a KML with one Folder per day.
- Creates Placemarks for each stop.
- Draws routes between stops (`--route straight` by default, or `--route osrm` for road routing via OSRM).
