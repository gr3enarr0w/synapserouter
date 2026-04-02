# v1.05 Search Backend: Tavily (AI-optimized search)
# Tests Tavily as the active search backend
Output tests/ui/recordings/v105-search-tavily.gif
Set Width 1200
Set Height 800
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env TAVILY_API_KEY "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"
Env BRAVE_API_KEY ""
Env SERPER_API_KEY ""
Env EXA_API_KEY ""
Env SEARXNG_URL ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-search-tavily-01-launch.png

Type "search the web for Kubernetes operator pattern"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-search-tavily-02-results.png
Sleep 10s
Screenshot tests/ui/screenshots/v105-search-tavily-03-done.png

Type "/exit"
Enter
Sleep 2s
