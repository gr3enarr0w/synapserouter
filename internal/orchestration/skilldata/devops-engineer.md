---
name: devops-engineer
description: "CI/CD pipelines, IaC, Terraform, Kubernetes, monitoring patterns."
triggers:
  - "ci/cd"
  - "pipeline"
  - "terraform"
  - "kubernetes"
  - "k8s"
  - "helm"
  - "ansible"
  - "jenkins"
  - "github actions"
  - "gitlab ci"
  - "infrastructure"
  - "deploy"
  - "monitoring"
role: coder
phase: implement
mcp_tools:
  - "context7.query-docs"
---
# Skill: DevOps Engineer

CI/CD pipelines, IaC, Terraform, Kubernetes, monitoring patterns.

Source: [DevOps CI/CD Engineer](https://mcpmarket.com/tools/skills/devops-ci-cd-engineer).

---

## When to Use

- Setting up CI/CD pipelines (GitHub Actions, Jenkins, GitLab CI)
- Infrastructure as Code (Terraform, Ansible)
- Kubernetes manifests and Helm charts
- Monitoring and alerting configuration

---

## Core Rules

1. **Everything as code** — infra, CI/CD, monitoring configs in git
2. **Immutable deployments** — never modify running containers/VMs
3. **Environment parity** — dev/staging/prod as similar as possible
4. **Secrets in vault** — never in code, env files, or CI variables
5. **Fail fast** — lint → test → build → deploy (stop on first failure)
6. **Blue-green or canary** — zero-downtime deployments

---

## GitHub Actions Pattern

```yaml
name: CI/CD
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.12" }
      - run: pip install -r requirements.txt
      - run: pytest --cov
  deploy:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: docker build -t app:${{ github.sha }} .
      - run: docker push app:${{ github.sha }}
```

## Terraform Pattern

```hcl
resource "google_cloud_run_service" "app" {
  name     = "jsm-reporting"
  location = "us-central1"

  template {
    spec {
      containers {
        image = "gcr.io/project/app:latest"
        env { name = "DB_URL"; value_from { secret_key_ref { name = "db-url" } } }
      }
    }
  }
}
```

---

## Monitoring Checklist

- [ ] Health check endpoints
- [ ] Structured logging (JSON)
- [ ] Metrics (request rate, latency, error rate)
- [ ] Alerting on error rate > threshold
- [ ] Dashboard for key metrics
