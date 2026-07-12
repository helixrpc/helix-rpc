import asyncio

try:
    import boto3
except ImportError:
    boto3 = None

class HelixKMS:
    def __init__(self, key_id: str, region_name: str = "us-east-1"):
        if boto3 is None:
            raise ImportError("boto3 is required for HelixKMS")
        self.key_id = key_id
        self.kms_client = boto3.client('kms', region_name=region_name)

    async def encrypt_payload(self, plaintext: bytes) -> bytes:
        loop = asyncio.get_event_loop()
        
        def _encrypt():
            response = self.kms_client.encrypt(
                KeyId=self.key_id,
                Plaintext=plaintext
            )
            return response['CiphertextBlob']
        
        return await loop.run_in_executor(None, _encrypt)

    async def decrypt_payload(self, ciphertext: bytes) -> bytes:
        loop = asyncio.get_event_loop()
        
        def _decrypt():
            response = self.kms_client.decrypt(
                CiphertextBlob=ciphertext,
                KeyId=self.key_id
            )
            return response['Plaintext']
            
        return await loop.run_in_executor(None, _decrypt)
