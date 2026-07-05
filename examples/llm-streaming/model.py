try:
    from transformers import AutoTokenizer, AutoModelForCausalLM
    import torch
    _HF_AVAILABLE = True
except ImportError:
    _HF_AVAILABLE = False

class LLMModel:
    def __init__(self):
        if _HF_AVAILABLE:
            print("Loading GPT-2 from HuggingFace...")
            self.tokenizer = AutoTokenizer.from_pretrained("gpt2")
            self.model = AutoModelForCausalLM.from_pretrained("gpt2")
            self.model.eval()
            print("GPT-2 loaded.")
        else:
            print("[Demo mode] transformers not installed — using simulated streaming.")

    def generate_stream(self, prompt: str):
        if _HF_AVAILABLE:
            # Real HuggingFace streaming
            inputs = self.tokenizer(prompt, return_tensors="pt")
            input_ids = inputs["input_ids"]
            with torch.no_grad():
                for _ in range(100):
                    outputs = self.model(input_ids)
                    next_token = outputs.logits[:, -1, :].argmax(dim=-1)
                    token_str = self.tokenizer.decode(next_token[0])
                    input_ids = torch.cat([input_ids, next_token.unsqueeze(0)], dim=-1)
                    yield token_str
                    if next_token.item() == self.tokenizer.eos_token_id:
                        break
        else:
            # Demo mode: realistic multi-sentence response
            import time
            responses = {
                "default": "Helix RPC is a next-generation AI serving framework that eliminates serialization overhead between your model and the network layer. By embedding Python directly inside the Rust gateway via PyO3, tokens flow from your model's output buffer straight into the HTTP response stream — zero copies, zero hops. This means the first token arrives at the client in microseconds after your model generates it, not milliseconds."
            }
            # Pick the response based on keywords in prompt
            text = responses["default"]
            words = text.split(" ")
            for i, word in enumerate(words):
                time.sleep(0.06)
                yield word + (" " if i < len(words) - 1 else "")
