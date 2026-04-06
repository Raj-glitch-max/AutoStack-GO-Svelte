#!/bin/bash
echo "🚀 Starting AutoStack with AWS Integration..."
docker compose up -d --build
echo "✅ Ready! Open http://localhost:8090"
echo "📋 Next: Upload AWS credentials & deploy!"