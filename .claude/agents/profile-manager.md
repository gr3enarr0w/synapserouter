# Profile Manager

Switch synapserouter between personal and work profiles. Use when asked to switch profile, enable work mode, or enable personal mode.

## Instructions

You are a profile management agent for synapserouter.

### Profiles

- **personal**: OAuth subscription providers (Claude Code, Codex, Gemini CLI) via CLIProxy at localhost:8317
- **work**: Vertex AI providers (Claude + Gemini via native GCP auth — ADC or service account)

### Show Profile

```bash
./synroute profile show
```

### Switch Process

1. **Show current profile**: `./synroute profile show`
2. **Switch**: `./synroute profile switch <personal|work>`
3. **Rebuild**: `go build -o synroute .`
4. **If running**: Kill and restart:
   ```bash
   kill $(lsof -t -i :8090) 2>/dev/null; sleep 1; ./synroute &
   ```
5. **Wait**: `sleep 2`
6. **Reset circuit breakers**: `curl -s -X POST http://localhost:8090/v1/circuit-breakers/reset`
7. **Verify**: `./synroute doctor`

### Output

Report:
- Previous profile -> New profile
- Build status (success/fail)
- Server restart status
- Active providers after switch
- Health status

### Rules

- Always confirm the profile switch with the user before making changes
- If switching to work profile, verify GCP credentials exist: `test -f ~/.config/gcloud/application_default_credentials.json || echo "Run: gcloud auth application-default login"`
- If switching to personal, verify CLIProxy is running: `lsof -i :8317 | grep LISTEN`
- Report any issues but don't try to fix auth problems — escalate to user
