# AutoStack Documentation

Complete documentation for AutoStack.

## Quick Links

### Getting Started
- **[Setup Guide](SETUP.md)** - Installation and configuration
- **[User Guide](../USER_GUIDE.md)** - How to use AutoStack
- **[Quick Start](SETUP.md#quick-start)** - Get running in 5 minutes

### Development
- **[Contributing](../CONTRIBUTING.md)** - Development guidelines
- **[Project Structure](PROJECT_STRUCTURE.md)** - Code organization
- **[API Reference](api/API.md)** - REST API documentation

### Deployment
- **[Deployment Guide](DEPLOYMENT.md)** - Production deployment
- **[Security](../SECURITY.md)** - Security best practices

## Documentation Structure

```
docs/
├── README.md                    # This file
├── SETUP.md                     # Setup instructions
├── DEPLOYMENT.md                # Deployment guide
├── PROJECT_STRUCTURE.md         # Project organization
├── PROJECT_STATUS.md            # Current project status
├── API_INTEGRATIONS.md          # External API guide (Infracost, Resend)
├── IMPLEMENTATION_SUMMARY.md    # Implementation summary
├── VERIFICATION_CHECKLIST.md    # Production readiness checklist
├── CLEANUP_SUMMARY.md           # Documentation cleanup log
├── PROJECT_STATUS_VERIFICATION.md  # Feature verification
├── api/
│   └── API.md                  # REST API reference
├── internal/                    # Internal technical docs
│   ├── README.md               # Internal docs index
│   ├── USAGE_ASSUMPTIONS_README.md
│   ├── COST_ESTIMATE_INTEGRATION.md
│   ├── ACTUAL_COST_FETCHER_README.md
│   ├── ACTUAL_COST_IMPLEMENTATION_SUMMARY.md
│   ├── ACTUAL_COST_JOB_README.md
│   ├── CACHE_IMPLEMENTATION.md
│   ├── IMPLEMENTATION_SUMMARY.md
│   └── CostEstimator_README.md
└── archive/                     # Historical documentation
    └── (35 archived files)
```

## Documentation by Role

### For Users
1. Start with [Setup Guide](SETUP.md)
2. Read [User Guide](../USER_GUIDE.md)
3. Check [API Documentation](api/API.md) if using API

### For Contributors
1. Read [Contributing Guidelines](../CONTRIBUTING.md)
2. Review [Project Structure](PROJECT_STRUCTURE.md)
3. Check [Internal Docs](internal/) for specific components

### For DevOps
1. Read [Deployment Guide](DEPLOYMENT.md)
2. Review [Security Policy](../SECURITY.md)
3. Check [Setup Guide](SETUP.md) for configuration

## Documentation Standards

### File Naming
- Use UPPERCASE for main docs (README.md, SETUP.md)
- Use lowercase for subdirectories (api/, internal/)
- Use descriptive names (DEPLOYMENT.md not DEPLOY.md)

### Content Structure
- Start with overview/purpose
- Include table of contents for long docs
- Use code examples
- Link to related documentation

### Maintenance
- Update docs with code changes
- Archive outdated documentation
- Keep examples current
- Review quarterly

## Getting Help

### Documentation Issues
- Missing information? [Open an issue](https://github.com/Raj-glitch-max/AutoStack/issues)
- Found an error? [Submit a PR](https://github.com/Raj-glitch-max/AutoStack/pulls)
- Have a question? [Start a discussion](https://github.com/Raj-glitch-max/AutoStack/discussions)

### Support Channels
- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - Questions and community support
- **Documentation** - This directory

## Contributing to Documentation

### How to Contribute
1. Fork the repository
2. Create a branch (`docs/your-improvement`)
3. Make your changes
4. Submit a pull request

### Documentation Guidelines
- Write in clear, simple English
- Include code examples
- Test all commands and code
- Update table of contents
- Link to related docs

### What to Document
- New features
- API changes
- Configuration options
- Common issues
- Best practices

## Version History

### v1.0.0 (2026-04-12)
- Initial documentation structure
- Setup, deployment, and API guides
- Internal technical documentation
- Archived historical docs

## License

Documentation is licensed under [Apache 2.0](../LICENSE), same as the code.

---

**Need help?** Check the [Setup Guide](SETUP.md) or [open an issue](https://github.com/Raj-glitch-max/AutoStack/issues).
