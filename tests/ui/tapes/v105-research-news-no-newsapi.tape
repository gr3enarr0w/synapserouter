# Test: News query with NO news backends — should fallback to web
# Verifies: news query classification falls back to general web search
Output tests/ui/recordings/v105-research-news-no-newsapi.gif
Set Width 1200
Set Height 800

Env NEWSAPI_KEY ""
Env NEWSDATA_API_KEY ""
Env THENEWSAPI_KEY ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/research quick latest Docker security update announced 2026"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-research-news-no-newsapi-01.png

Type "/exit"
Enter
Sleep 2s
