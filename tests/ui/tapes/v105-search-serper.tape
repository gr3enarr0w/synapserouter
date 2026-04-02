# v1.05 Search Backend: Serper (Google results)
# Tests Serper as the active search backend
Output tests/ui/recordings/v105-search-serper.gif
Set Width 1200
Set Height 800
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env SERPER_API_KEY "c26629fcd2604ed776d54bc486aec5697d17f9fd"
Env BRAVE_API_KEY ""
Env TAVILY_API_KEY ""
Env EXA_API_KEY ""
Env SEARXNG_URL ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-search-serper-01-launch.png

Type "search the web for Go error handling patterns"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-search-serper-02-results.png
Sleep 10s
Screenshot tests/ui/screenshots/v105-search-serper-03-done.png

Type "/exit"
Enter
Sleep 2s
