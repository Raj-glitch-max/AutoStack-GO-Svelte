# AutoStack User Guide - Getting Started

## What is AutoStack?

AutoStack is a platform that lets you deploy applications to Kubernetes or AWS with just a few clicks. Think of it as Heroku/Vercel but for any containerized application.

## 🚀 Quick Start (5 Minutes)

### Step 1: Create Your Account
1. Open http://localhost:5173 in your browser
2. Click "Sign Up" 
3. Enter your email and password
4. You're in!

### Step 2: Create Your First Project
1. Click "New Project" button
2. Give it a name (e.g., "My First App")
3. Add a description (optional)
4. Click "Create"

### Step 3: Deploy Something (The Magic Part!)
1. Inside your project, click "New Deployment"
2. You'll see two options:
   - **Kubernetes Deployment** (Local/Simple) ✅ Ready Now
   - **AWS Deployment** (Cloud/Production) ⚠️ Needs AWS Setup

### For Kubernetes (Easiest - Try This First):
1. Choose "Kubernetes Deployment"
2. Select a blueprint:
   - **NGINX Web Server** - Simple web server (recommended for first try)
   - **Node.js Application** - Node.js app with config
3. Give it a name (e.g., "my-nginx")
4. Click "Deploy"
5. Watch it deploy in real-time! 🎉

You'll see:
- Deployment status (Creating → Running)
- Live logs from your containers
- Resource usage (CPU/Memory)
- Access URLs

---

## 🌩️ One-Click Cloud Deployment (AWS)

This is the advanced feature that was recently implemented. Here's what it does:

### What's Been Implemented:

#### 1. **AWS Credentials Management**
- Securely store AWS Access Keys (encrypted in database)
- Support for multiple AWS accounts
- Automatic credential validation

#### 2. **AWS Blueprints** (Pre-configured Infrastructure)
Available blueprints:
- **Static Website** - S3 + CloudFront CDN
- **Web Application** - ECS Fargate + ALB + RDS
- **Full-Stack App** - Frontend (S3) + Backend (ECS) + Database (RDS)
- **Microservices** - ECS cluster with service mesh
- **Serverless API** - Lambda + API Gateway + DynamoDB

#### 3. **Terraform Integration**
- Automatic infrastructure-as-code generation
- State management in S3
- Real-time deployment logs
- Automatic rollback on failure

#### 4. **Cost Estimation**
- Shows estimated monthly AWS costs BEFORE you deploy
- Breaks down costs by service (EC2, RDS, S3, etc.)
- Helps you stay within budget

#### 5. **Deployment Tracking**
- Real-time Terraform execution logs
- Deployment status tracking
- Resource inventory (what got created)
- One-click destroy when done

---

## 📋 How to Use AWS Deployment

### Prerequisites:
1. AWS Account with billing enabled
2. AWS Access Key ID and Secret Access Key
3. S3 bucket for Terraform state (optional - can be created)

### Step-by-Step:

#### 1. Set Up AWS Credentials
```
Go to: Settings → AWS Credentials → Add New
- Name: "My AWS Account"
- Access Key ID: [your key]
- Secret Access Key: [your secret]
- Default Region: us-east-1
- Click "Save & Test"
```

#### 2. Create AWS Deployment
```
Project → New Deployment → AWS Deployment
- Select Blueprint: "Static Website" (easiest)
- Name: "my-static-site"
- Select AWS Credentials
- Review Cost Estimate
- Click "Deploy to AWS"
```

#### 3. Watch It Deploy
- Terraform runs automatically
- Creates: S3 bucket, CloudFront distribution, Route53 records
- Shows real-time logs
- Gives you the website URL when done

#### 4. Manage Deployment
- View resources created
- Check deployment logs
- Update configuration
- Destroy when done (one click)

---

## 🎯 Key Features Implemented

### Security (Phase 1 - Week 1)
✅ Encrypted AWS credentials storage (AES-256)
✅ Input sanitization (prevents injection attacks)
✅ Ownership verification (users can only see their own stuff)
✅ WebSocket authentication for real-time logs

### Reliability (Phase 1 - Week 2-3)
✅ Deployment confirmation gate (prevents accidents)
✅ Auto-destroy on timeout (no forgotten resources)
✅ Concurrent deployment prevention (one at a time)
✅ Graceful shutdown handling
✅ Automatic cleanup of failed deployments
✅ Log rotation and streaming

### User Experience
✅ Real-time deployment logs
✅ Cost estimation before deployment
✅ Blueprint templates (no infrastructure knowledge needed)
✅ One-click deploy and destroy
✅ Visual deployment status

---

## 🏗️ Architecture Overview

```
┌─────────────┐
│   Browser   │ ← You interact here
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  Frontend   │ ← Svelte UI (http://localhost:5173)
│  (Vite)     │
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  Backend    │ ← Go + PocketBase (http://localhost:8090)
│ (PocketBase)│
└──────┬──────┘
       │
       ├─→ Kubernetes API ← For local deployments
       │   (minikube)
       │
       └─→ AWS API ← For cloud deployments
           (via Terraform)
```

---

## 📁 What Each Part Does

### Frontend (`/frontend`)
- User interface (what you see)
- Forms for creating projects/deployments
- Real-time log viewer
- Cost calculator
- Deployment status dashboard

### Backend (`/pocketbase`)
- API server
- Database (SQLite)
- Kubernetes controller (talks to K8s)
- AWS controller (talks to AWS)
- Terraform executor (runs infrastructure code)
- WebSocket server (real-time updates)

### Blueprints (`/docs/blueprints`)
- Pre-made deployment templates
- YAML files with Kubernetes configs
- Terraform templates for AWS

---

## 🎬 Demo Scenarios

### Scenario 1: "I want to deploy a simple website"
1. Create account
2. Create project: "My Website"
3. New Deployment → Kubernetes → NGINX
4. Access at the provided URL
5. Done in 2 minutes!

### Scenario 2: "I want to deploy to AWS production"
1. Set up AWS credentials (one time)
2. Create project: "Production App"
3. New Deployment → AWS → Static Website
4. Review $5/month cost estimate
5. Click Deploy
6. Get CloudFront URL in 5 minutes
7. Your site is live on AWS!

### Scenario 3: "I want to deploy a full-stack app"
1. Create project: "Full Stack App"
2. New Deployment → AWS → Full-Stack Blueprint
3. Configure:
   - Frontend: React/Vue/etc
   - Backend: Node.js/Python/etc
   - Database: PostgreSQL/MySQL
4. Review $50/month cost estimate
5. Deploy
6. Get: Frontend URL, API URL, Database endpoint

---

## 🔍 Where to Find Things in the UI

### Main Navigation:
- **Projects** - List all your projects
- **Blueprints** - Browse available templates
- **Deployments** - See all active deployments
- **Settings** - AWS credentials, profile

### Inside a Project:
- **Overview** - Project details
- **Deployments** - List of deployments in this project
- **New Deployment** - Deploy something new

### Inside a Deployment:
- **Status** - Current state (Running/Failed/etc)
- **Logs** - Real-time container/Terraform logs
- **Resources** - What got created
- **Metrics** - CPU/Memory usage (Kubernetes)
- **Actions** - Restart, Scale, Destroy

---

## 💡 Tips

1. **Start with Kubernetes** - It's local, free, and instant
2. **Try NGINX first** - Simplest blueprint, deploys in seconds
3. **Check cost estimates** - Before deploying to AWS
4. **Use blueprints** - Don't write YAML/Terraform yourself
5. **Destroy when done** - Avoid AWS charges

---

## 🐛 Troubleshooting

### "Deployment stuck in Creating"
- Check logs tab for errors
- Verify Kubernetes cluster is running: `kubectl get nodes`
- Check backend logs: Process 3

### "AWS deployment failed"
- Verify AWS credentials are correct
- Check you have permissions (IAM)
- Review Terraform logs in deployment

### "Can't access deployed app"
- Kubernetes: Check service type (ClusterIP vs LoadBalancer)
- AWS: Wait for DNS propagation (5-10 minutes)

---

## 📚 Next Steps

1. **Try the demo** - Follow Scenario 1 above
2. **Explore blueprints** - See what's available
3. **Set up AWS** - If you want cloud deployments
4. **Create custom blueprints** - Advanced users

---

## 🎯 What Makes This Special

Traditional way to deploy:
1. Learn Kubernetes YAML (weeks)
2. Learn Terraform (weeks)
3. Set up CI/CD (days)
4. Configure monitoring (days)
5. Deploy (hours)

**AutoStack way:**
1. Click "Deploy"
2. Done (minutes)

That's the magic! 🪄
