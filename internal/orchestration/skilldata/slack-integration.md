---
name: slack-integration
description: "Slack channel operations, message posting, signal extraction via API or MCP."
triggers:
  - "slack"
  - "channel"
  - "post message"
  - "slack notification"
role: coder
phase: implement
---
# Skill: Slack Integration

Slack channel operations, message posting, signal extraction via API or MCP.

Source: [Slack Agent Integration](https://mcpmarket.com/tools/skills/slack-agent-channel-integration), [Slack MCP](https://slack.com/help/articles/48855576908307).

---

## When to Use

- Posting messages or notifications to Slack
- Reading channel history for data extraction
- Building Slack-integrated workflows
- Monitoring channels for signals

---

## Core Rules

1. **Bot tokens** — use `xoxb-` tokens for app-level access
2. **Scopes** — request minimum scopes (`channels:read`, `chat:write`)
3. **Rate limits** — Slack rate-limits aggressively, batch and throttle
4. **Threading** — reply in threads to avoid channel noise
5. **Blocks API** — use Block Kit for rich message formatting

---

## API Patterns

### Post a message
```bash
curl -s -X POST https://slack.com/api/chat.postMessage \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "channel": "C0123456789",
    "text": "New ticket classified: JIRACONFSD-100",
    "blocks": [
      {"type": "header", "text": {"type": "plain_text", "text": "New Ticket Alert"}},
      {"type": "section", "text": {"type": "mrkdwn", "text": "*JIRACONFSD-100*: SSO login failing"}}
    ]
  }'
```

### Read channel history
```bash
curl -s "https://slack.com/api/conversations.history?channel=C0123456789&limit=100" \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN"
```

### Get thread replies
```bash
curl -s "https://slack.com/api/conversations.replies?channel=C0123456789&ts=1234567890.123456" \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN"
```

---

## MCP Usage (when configured)

When the Slack MCP is available, use MCP tools instead of raw API calls for:
- Channel listing and search
- Message posting with blocks
- Thread management
- User lookup
