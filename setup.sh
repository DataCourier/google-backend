#!/bin/bash
set -e

echo "🚀 Backend Setup Script"
echo ""

# Check if config.yaml exists
if [ ! -f "config.yaml" ]; then
    echo "❌ config.yaml not found!"
    echo "Please copy config.example.yaml to config.yaml and edit it."
    exit 1
fi

# Parse config.yaml (requires yq or we'll do simple grep/awk)
# For now, simple parsing. TODO: Use yq for robust YAML parsing

PROJECT_NAME=$(grep "name:" config.yaml | head -1 | awk '{print $2}' | tr -d '"')
MODULE_PATH=$(grep "module_path:" config.yaml | awk '{print $2}' | tr -d '"')
FIREBASE_PROJECT=$(grep "firebase_project_id:" config.yaml | awk '{print $2}' | tr -d '"')

echo "📋 Configuration:"
echo "   Project Name: $PROJECT_NAME"
echo "   Module Path:  $MODULE_PATH"
echo "   Firebase:     $FIREBASE_PROJECT"
echo ""

# Function to sync shared code to a service
sync_shared_to_service() {
    local service_dir=$1
    echo "📦 Syncing shared code to $service_dir..."

    # Create directories if they don't exist
    mkdir -p "$service_dir/auth"
    mkdir -p "$service_dir/middleware"
    mkdir -p "$service_dir/response"

    # Copy shared code
    cp -r shared/auth/* "$service_dir/auth/" 2>/dev/null || true
    cp -r shared/middleware/* "$service_dir/middleware/" 2>/dev/null || true
    cp -r shared/response/* "$service_dir/response/" 2>/dev/null || true

    echo "   ✅ Shared code synced"
}

# Function to update go.mod in a service
update_go_mod() {
    local service_dir=$1
    local service_name=$(basename "$service_dir")
    local go_mod_path="$service_dir/go.mod"

    echo "📝 Updating go.mod for $service_name..."

    if [ -f "$go_mod_path" ]; then
        # Update module path
        sed -i "s|module .*|module $MODULE_PATH/services/$service_name|g" "$go_mod_path"
        echo "   ✅ Module path updated"
    else
        echo "   ⚠️  go.mod not found, skipping"
    fi
}

# Sync all enabled services
echo "🔄 Syncing services..."
for service_dir in services/*; do
    if [ -d "$service_dir" ]; then
        service_name=$(basename "$service_dir")
        echo ""
        echo "📂 Processing $service_name..."

        sync_shared_to_service "$service_dir"
        update_go_mod "$service_dir"
    fi
done

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Test locally:  cd services/user-service && go run main.go"
echo "  2. Deploy:        make deploy"
echo "  3. Docs:          See README.md"
