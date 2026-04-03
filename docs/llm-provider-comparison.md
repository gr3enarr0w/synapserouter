# LLM Provider Support Comparison (2026)

## Overview
Comparison of LLM provider support across major AI coding agents, including synroute.

## Provider Support Matrix

| Provider | OpenCode | Aider | Cline | Continue.dev | Cursor | synroute |
|----------|----------|-------|-------|-------------|--------|----------|
| **OpenAI** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Anthropic** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Google Gemini** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Ollama** | ✅ | ✅ | ❌ | ✅ | ❌ | ✅ |
| **Azure AI** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **AWS Bedrock** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Cohere** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **DeepSeek** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **xAI** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **GROQ** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **OpenRouter** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Vertex AI** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **LM Studio** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ |
| **GitHub Copilot** | ❌ | ✅ | ❌ | ❌ | ✅ | ❌ |
| **Cerebras** | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| **Any OpenAI-compatible** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Key Observations

### synroute Strengths
- ✅ Excellent coverage of core providers (OpenAI, Anthropic, Google Gemini, Vertex AI)
- ✅ Strong local model support via Ollama integration
- ✅ Support for OpenAI-compatible APIs
- ✅ Multi-provider fallback chain with circuit breakers

### synroute Gaps
- ❌ Missing Azure AI support
- ❌ Missing AWS Bedrock support  
- ❌ Missing OpenRouter integration
- ❌ Missing several specialty providers (Cohere, DeepSeek, xAI, GROQ, LM Studio)

### Industry Trends (2026)
1. **Provider Diversity**: All major tools now support core commercial providers
2. **Local Model Support**: Ollama and local APIs becoming standard
3. **Gateway Integration**: Tools using LLM gateways for enhanced capabilities
4. **Comprehensive Coverage**: Leading tools support 10+ providers via compatibility layers

## Recommended Enhancements for synroute

### Priority 1 (Core Business Providers)
1. **Azure AI** - Enterprise integration
2. **AWS Bedrock** - Cloud provider coverage
3. **OpenRouter** - Access to 100+ models

### Priority 2 (Specialty Providers)
1. **Cohere** - Command models for coding
2. **DeepSeek** - Strong coding capabilities
3. **GROQ** - High-speed inference

### Priority 3 (Developer Experience)
1. **LM Studio** - Local model management
2. **GitHub Copilot** - Microsoft ecosystem integration

## Implementation Notes

synroute currently excels in:
- Multi-provider fallback chains
- Circuit breaker patterns
- Local model support via Ollama
- Vertex AI enterprise integration

Areas for improvement:
- Expanded commercial provider coverage
- Gateway service integration
- Specialty model provider support

This comparison shows synroute is well-positioned but could benefit from expanded provider support to match industry leaders like OpenCode and Aider.