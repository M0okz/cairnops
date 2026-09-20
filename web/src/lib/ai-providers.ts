/** Curated Chat Completions + JSON-mode presets; custom endpoints/models remain supported. */
export const aiProviders = [
  {
    id: "gemini",
    name: "Gemini (Google)",
    endpoint: "https://generativelanguage.googleapis.com/v1beta/openai",
    models: [
      { id: "gemini-3.8-flash", name: "Gemini 3.8 Flash" },
      { id: "gemini-3.1-flash-lite", name: "Gemini 3.1 Flash-Lite" },
      { id: "gemini-2.5-flash", name: "Gemini 2.5 Flash" },
      { id: "gemini-2.5-pro", name: "Gemini 2.5 Pro" },
    ],
  },
  {
    id: "openai",
    name: "OpenAI",
    endpoint: "https://api.openai.com/v1",
    models: [
      { id: "gpt-4.1-mini", name: "GPT-4.1 mini" },
      { id: "gpt-4.1", name: "GPT-4.1" },
    ],
  },
  {
    id: "mistral",
    name: "Mistral",
    endpoint: "https://api.mistral.ai/v1",
    models: [
      { id: "mistral-small-latest", name: "Mistral Small" },
      { id: "mistral-large-latest", name: "Mistral Large" },
    ],
  },
  {
    id: "deepseek",
    name: "DeepSeek",
    endpoint: "https://api.deepseek.com/v1",
    models: [{ id: "deepseek-chat", name: "DeepSeek Chat" }],
  },
];

export function providerForEndpoint(endpoint: string) {
  const normalized = endpoint.replace(/\/+$/, "");
  return aiProviders.find(
    (provider) =>
      provider.endpoint === normalized ||
      (provider.id === "deepseek" && normalized === "https://api.deepseek.com"),
  );
}
