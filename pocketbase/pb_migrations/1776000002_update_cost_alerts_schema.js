migrate((db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("cost_alerts");

  // Update 'type' field values
  const typeField = collection.schema.getFieldByName("type");
  if (typeField) {
    typeField.options.values = [
      "SPIKE",
      "DROP",
      "cost_overrun",
      "budget_exceeded",
      "anomaly_detected"
    ];
  }

  // Add 'serviceBreakdown' JSON field if it doesn't exist
  try {
    collection.schema.getFieldByName("serviceBreakdown");
  } catch (e) {
    collection.schema.addField(new SchemaField({
      "id": "service_breakdown_field",
      "name": "serviceBreakdown",
      "type": "json",
      "system": false,
      "required": false,
      "options": {}
    }));
  }

  return dao.saveCollection(collection);
}, (db) => {
  const dao = new Dao(db);
  const collection = dao.findCollectionByNameOrId("cost_alerts");

  // Revert 'type' field values
  const typeField = collection.schema.getFieldByName("type");
  if (typeField) {
    typeField.options.values = [
      "cost_overrun",
      "budget_exceeded",
      "anomaly_detected"
    ];
  }

  // Remove 'serviceBreakdown' field
  try {
    collection.schema.removeFieldByName("serviceBreakdown");
  } catch (e) {
    // Field might not exist
  }

  return dao.saveCollection(collection);
})
