# v1.05 Search Backend: Exa (semantic/neural search)
# Tests Exa as the active search backend
Output tests/ui/recordings/v105-search-exa.gif
Set Width 1200
Set Height 800
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env EXA_API_KEY "b7a65e36-5c38-4eb7-93ec-0d88a719fafb"
Env BRAVE_API_KEY ""
Env SERPER_API_KEY ""
Env TAVILY_API_KEY ""
Env SEARXNG_URL ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-search-exa-01-launch.png

Type "search the web for Python dataclass best practices"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-search-exa-02-results.png
Sleep 10s
Screenshot tests/ui/screenshots/v105-search-exa-03-done.png

Type "/exit"
Enter
Sleep 2s
