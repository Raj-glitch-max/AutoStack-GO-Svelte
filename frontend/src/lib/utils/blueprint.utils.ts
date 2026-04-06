export function recordLogoUrl(record: any): string {
    if (!record) {
        return "";
    }
    
    // Check for logo field (blueprints) or avatar field (users)
    const logoField = record.logo || record.avatar;
    if (!logoField) {
        return "";
    }

    return "/api/files/" + record?.collectionId + "/" + record?.id + "/" + logoField;
}
