import asyncio
import json


class AgenticStream:
    def __init__(self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter):
        self.reader = reader
        self.writer = writer
        self.call_id = 0

    async def send_token(self, token: str) -> None:
        """Sends a text token to the client."""
        payload = json.dumps({"type": "token", "content": token}) + "\n"
        self.writer.write(payload.encode("utf-8"))
        await self.writer.drain()

    async def call_tool(self, tool_name: str, args: dict) -> dict:
        """Invokes a client-side tool, suspends execution, and blocks waiting for the response."""
        self.call_id += 1
        current_id = self.call_id
        
        # Write tool call request frame
        payload = json.dumps({
            "type": "tool_call",
            "id": current_id,
            "name": tool_name,
            "arguments": args
        }) + "\n"
        self.writer.write(payload.encode("utf-8"))
        await self.writer.drain()

        # Block reading lines until we find the tool_response matching current_id
        while True:
            line = await self.reader.readline()
            if not line:
                raise EOFError("connection closed while waiting for tool response")
            
            try:
                frame = json.loads(line.decode("utf-8").strip())
                if frame.get("type") == "tool_response" and frame.get("id") == current_id:
                    return frame.get("result", {})
            except Exception:
                pass
