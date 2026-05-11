package aws

// This file contains test bootstrapping helpers for the aws package integration tests.
// It is NOT a _test.go file so it can be called from both test and non-test contexts,
// but is only used from test helpers. The bootstrap functions accept a minimal interface
// to avoid the TestApp vs *pocketbase.PocketBase type mismatch.

import (
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

// appWithDao is satisfied by both *pocketbase.PocketBase and *tests.TestApp
// since both expose Dao(). This avoids import-cycle issues in tests.
type appWithDao interface {
	Dao() *daos.Dao
}

// bootstrapPricingCacheCollection creates the awsPricingCache collection in the
// provided test PocketBase DB. tests.NewTestApp() starts with a blank schema, so any
// test that exercises DB round-trips must seed the required collections manually.
//
// The field set here must match what pricing_fetcher.go writes via record.Set().
func bootstrapPricingCacheCollection(app appWithDao) error {
	collection := &models.Collection{
		Name: "awsPricingCache",
		Type: models.CollectionTypeBase,
		Schema: schema.NewSchema(
			&schema.SchemaField{Name: "region", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "service", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "productFamily", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "instanceType", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "vcpu", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "memory", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "pricePerHour", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "pricePerMonth", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "currency", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "fetchedAt", Type: schema.FieldTypeDate},
			&schema.SchemaField{Name: "validUntil", Type: schema.FieldTypeDate},
		),
	}

	return app.Dao().SaveCollection(collection)
}
