# Test: Research with ONLY DuckDuckGo — absolute minimum config
# Verifies: works with single backend, no fusion, no crash
Output tests/ui/recordings/v105-research-one-backend.gif
Set Width 1200
Set Height 800

Env BRAVE_API_KEY ""
Env TAVILY_API_KEY ""
Env SERPER_API_KEY ""
Env EXA_API_KEY ""
Env YOU_API_KEY ""
Env JINA_API_KEY ""
Env KAGI_API_KEY ""
Env LINKUP_API_KEY ""
Env SERPAPI_KEY ""
Env SEARCHAPI_KEY ""
Env SOURCEGRAPH_TOKEN ""
Env GITHUB_TOKEN ""
Env NEWSAPI_KEY ""
Env NEWSDATA_API_KEY ""
Env THENEWSAPI_KEY ""
Env SEARXNG_URL ""

Type "cd /Users/ceverson/Development/synapserouter && ./synroute code"
Enter
Sleep 5s

Type "/research quick what is Kubernetes"
Enter
Sleep 20s
Screenshot tests/ui/screenshots/v105-research-one-backend-01.png

Type "/exit"
Enter
Sleep 2s
