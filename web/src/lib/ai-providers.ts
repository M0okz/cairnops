/** Curated Chat Completions + JSON-mode presets; custom endpoints/models remain supported. */
export const aiProviders = [
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
