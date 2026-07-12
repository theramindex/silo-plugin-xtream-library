package cache

import "github.com/theramindex/silo-plugin-xtream-library/internal/model"

func SnapshotFromCatalog(catalog model.CatalogState) Snapshot {
	return Snapshot{Catalog: catalog, Health: catalog.Health}
}
