# Contributing to AutoStack

Thank you for your interest in contributing to AutoStack! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting Started

### Development Setup

1. **Fork and Clone**
   ```bash
   git clone https://github.com/YOUR_USERNAME/AutoStack.git
   cd AutoStack
   ```

2. **Install Dependencies**
   ```bash
   # Backend
   cd pocketbase
   go mod download
   
   # Frontend
   cd ../frontend
   npm install
   ```

3. **Configure Environment**
   ```bash
   cp .env.example .env
   # Add your API keys
   ```

4. **Run Development Servers**
   ```bash
   # Backend (terminal 1)
   cd pocketbase
   go run main.go serve
   
   # Frontend (terminal 2)
   cd frontend
   npm run dev
   ```

## Development Guidelines

### Code Style

**Go:**
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Run `go vet` before committing
- Add comments for exported functions

**TypeScript/Svelte:**
- Use Prettier for formatting
- Follow ESLint rules
- Use TypeScript for type safety
- Add JSDoc comments for complex functions

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add cost estimation caching
fix: resolve deployment timeout issue
docs: update API documentation
test: add unit tests for cost calculator
refactor: simplify error handling logic
```

### Branch Naming

- `feature/description` - New features
- `fix/description` - Bug fixes
- `docs/description` - Documentation updates
- `refactor/description` - Code refactoring

## Pull Request Process

1. **Create a Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Changes**
   - Write clean, documented code
   - Add tests for new features
   - Update documentation

3. **Test Your Changes**
   ```bash
   # Backend tests
   cd pocketbase
   go test ./...
   
   # Frontend tests
   cd frontend
   npm test
   ```

4. **Commit and Push**
   ```bash
   git add .
   git commit -m "feat: your feature description"
   git push origin feature/your-feature-name
   ```

5. **Create Pull Request**
   - Go to GitHub and create a PR
   - Fill out the PR template
   - Link related issues
   - Request review

### PR Requirements

- [ ] Code follows style guidelines
- [ ] Tests pass
- [ ] Documentation updated
- [ ] No merge conflicts
- [ ] Reviewed by maintainer

## Testing

### Backend Tests

```bash
cd pocketbase
go test ./...
go test -race ./...  # Check for race conditions
go test -cover ./... # Check coverage
```

### Frontend Tests

```bash
cd frontend
npm test
npm run test:coverage
```

### Integration Tests

```bash
cd pocketbase
go run test_integration.go
```

## Documentation

### Code Documentation

- Add comments for all exported functions
- Include examples in comments
- Document complex algorithms
- Update README when adding features

### API Documentation

Update `docs/api/API.md` when:
- Adding new endpoints
- Changing request/response formats
- Modifying authentication

## Project Structure

```
AutoStack/
├── pocketbase/          # Backend (Go + PocketBase)
│   ├── pkg/            # Go packages
│   ├── pb_migrations/  # Database migrations
│   └── main.go         # Entry point
├── frontend/           # Frontend (SvelteKit)
│   ├── src/           # Source code
│   └── static/        # Static assets
├── deployment/        # Kubernetes manifests
├── docs/             # Documentation
│   ├── api/          # API docs
│   ├── guides/       # User guides
│   └── archive/      # Old docs
└── scripts/          # Utility scripts
```

## Adding New Features

### Backend Feature

1. Create package in `pocketbase/pkg/`
2. Add tests in `*_test.go`
3. Register routes in `main.go`
4. Update API documentation

### Frontend Feature

1. Create component in `frontend/src/lib/components/`
2. Add route in `frontend/src/routes/`
3. Update navigation if needed
4. Add tests

### Blueprint

1. Create template in `pocketbase/templates/`
2. Add cost calculator in `pocketbase/pkg/cost/`
3. Update blueprint mapper
4. Add documentation

## Reporting Bugs

### Before Reporting

- Check existing issues
- Try latest version
- Gather error logs
- Create minimal reproduction

### Bug Report Template

```markdown
**Describe the bug**
A clear description of the bug.

**To Reproduce**
Steps to reproduce:
1. Go to '...'
2. Click on '...'
3. See error

**Expected behavior**
What you expected to happen.

**Screenshots**
If applicable, add screenshots.

**Environment:**
- OS: [e.g. Ubuntu 22.04]
- Browser: [e.g. Chrome 120]
- Version: [e.g. 1.0.0]

**Additional context**
Any other relevant information.
```

## Feature Requests

### Feature Request Template

```markdown
**Is your feature request related to a problem?**
A clear description of the problem.

**Describe the solution you'd like**
A clear description of what you want to happen.

**Describe alternatives you've considered**
Other solutions you've thought about.

**Additional context**
Any other relevant information.
```

## Community

- **GitHub Discussions**: Ask questions, share ideas
- **GitHub Issues**: Report bugs, request features
- **Pull Requests**: Contribute code

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

Feel free to ask questions in:
- GitHub Discussions
- GitHub Issues (with `question` label)

Thank you for contributing to AutoStack! 🚀
