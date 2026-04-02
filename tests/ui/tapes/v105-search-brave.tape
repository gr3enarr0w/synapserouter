# v1.05 Search Backend: Brave (independent web index)
# Tests Brave as the active search backend
Output tests/ui/recordings/v105-search-brave.gif
Set Width 1200
Set Height 800
Set Shell "bash"
Set FontSize 14
Set TypingSpeed 50ms

Env BRAVE_API_KEY "BSAp8bt0p2XJITWW1ukjMujQCdAsfmb"
Env SERPER_API_KEY ""
Env TAVILY_API_KEY ""
Env EXA_API_KEY ""
Env SEARXNG_URL ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s
Screenshot tests/ui/screenshots/v105-search-brave-01-launch.png

Type "search the web for Rust async runtime comparison"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-search-brave-02-results.png
Sleep 10s
Screenshot tests/ui/screenshots/v105-search-brave-03-done.png

Type "/exit"
Enter
Sleep 2s
