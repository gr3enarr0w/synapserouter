# Skill: SynapseRouter Profile Management

## Triggers
Use this skill when the user says: "switch profile", "work mode", "personal mode", "show profile", "which profile", "switch to work", "switch to personal", "enable work", "enable personal"

## Process

### Show Current Profile
If the user asks which profile is active:
1. Run `./synroute profile show`

### List Profiles
1. Run `./synroute profile list`

### Switch Profile
If the user asks to switch profiles:
1. Delegate to `@profile-manager` subagent
2. The subagent handles: profile switch, rebuild, restart, verification
