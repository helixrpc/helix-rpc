import time
import asyncio
import grpc
from concurrent import futures
import chat_completion_pb2
import chat_completion_pb2_grpc

class ChatCompletionServiceServicer(chat_completion_pb2_grpc.ChatCompletionServiceServicer):
    def CreateChatCompletion(self, request, context):
        # A simple dummy response for unary calls
        reply_message = chat_completion_pb2.ChatMessage(
            role="assistant",
            content=f"Echo (unary): you said {request.messages[-1].content if request.messages else ''}"
        )
        choice = chat_completion_pb2.ChatCompletionChoice(
            index=0,
            message=reply_message,
            finish_reason="stop"
        )
        return chat_completion_pb2.ChatCompletionResponse(
            id="chatcmpl-123",
            object="chat.completion",
            created=int(time.time()),
            model=request.model,
            choices=[choice]
        )

    def StreamChatCompletion(self, request, context):
        # A simple dummy streaming response
        words = ["Hello,", " I", " am", " your", " AI", " assistant.", " How", " can", " I", " help", " you?"]
        for i, word in enumerate(words):
            time.sleep(0.1) # Simulate generation latency
            delta = chat_completion_pb2.ChatMessage(
                role="assistant" if i == 0 else "",
                content=word
            )
            choice = chat_completion_pb2.ChatCompletionChoice(
                index=0,
                delta=delta,
                finish_reason="" if i < len(words) - 1 else "stop"
            )
            yield chat_completion_pb2.ChatCompletionResponse(
                id="chatcmpl-123",
                object="chat.completion.chunk",
                created=int(time.time()),
                model=request.model,
                choices=[choice]
            )

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    chat_completion_pb2_grpc.add_ChatCompletionServiceServicer_to_server(
        ChatCompletionServiceServicer(), server)
    server.add_insecure_port('[::]:50051')
    server.start()
    print("Python AI Model Server listening on port 50051...")
    server.wait_for_termination()

if __name__ == '__main__':
    serve()
