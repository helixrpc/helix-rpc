document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('chatForm');
    const input = document.getElementById('promptInput');
    const sendBtn = document.getElementById('sendBtn');
    const messagesContainer = document.getElementById('messagesContainer');
    const newChatBtn = document.getElementById('newChatBtn');

    // Auto-resize textarea
    input.addEventListener('input', function() {
        this.style.height = 'auto';
        this.style.height = (this.scrollHeight) + 'px';
        sendBtn.disabled = this.value.trim().length === 0;
    });

    // Handle Enter key (but Shift+Enter for new line)
    input.addEventListener('keydown', function(e) {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            if (!sendBtn.disabled) {
                form.dispatchEvent(new Event('submit'));
            }
        }
    });

    // New Chat
    newChatBtn.addEventListener('click', () => {
        messagesContainer.innerHTML = '';
        input.value = '';
        input.focus();
        input.style.height = 'auto';
        sendBtn.disabled = true;
    });

    // SVG icons
    const userIcon = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>`;
    const assistantIcon = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a10 10 0 1 0 10 10H12V2z"></path><path d="M12 12L2.1 7.1"></path><path d="M12 12l9.9 4.9"></path></svg>`;

    function appendMessage(role, content, isStreaming = false) {
        const msgDiv = document.createElement('div');
        msgDiv.className = `message ${role}`;
        
        const avatarDiv = document.createElement('div');
        avatarDiv.className = 'avatar';
        avatarDiv.innerHTML = role === 'user' ? userIcon : assistantIcon;

        const contentDiv = document.createElement('div');
        contentDiv.className = 'message-content';
        
        const p = document.createElement('p');
        p.textContent = content;
        
        if (isStreaming) {
            const cursor = document.createElement('span');
            cursor.className = 'cursor-blink';
            p.appendChild(cursor);
            // Attach a reference so we can easily append to it later
            contentDiv.pElement = p;
            contentDiv.cursor = cursor;
        }

        contentDiv.appendChild(p);
        msgDiv.appendChild(avatarDiv);
        msgDiv.appendChild(contentDiv);
        
        messagesContainer.appendChild(msgDiv);
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
        
        return contentDiv;
    }

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const prompt = input.value.trim();
        if (!prompt) return;

        // Reset input
        input.value = '';
        input.style.height = 'auto';
        sendBtn.disabled = true;
        
        // Append user message
        appendMessage('user', prompt);

        // Append empty assistant message for streaming
        const assistantContentNode = appendMessage('assistant', '', true);
        
        try {
            // Hit the Rust PyO3 Gateway!
            const response = await fetch('/v1/chat/completions', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Accept': 'text/event-stream'
                },
                body: JSON.stringify({
                    messages: [{ role: 'user', content: prompt }]
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder('utf-8');
            let buffer = '';

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                buffer += decoder.decode(value, { stream: true });
                const lines = buffer.split('\n');
                
                // Keep the last incomplete line in the buffer
                buffer = lines.pop();

                for (const line of lines) {
                    if (line.startsWith('data: ')) {
                        const dataStr = line.slice(6);
                        if (dataStr === '[DONE]') {
                            break;
                        }
                        
                        try {
                            const data = JSON.parse(dataStr);
                            if (data.choices && data.choices[0].delta && data.choices[0].delta.content) {
                                const textChunk = data.choices[0].delta.content;
                                // Append chunk text before the cursor
                                const p = assistantContentNode.pElement;
                                const textNode = document.createTextNode(textChunk);
                                p.insertBefore(textNode, assistantContentNode.cursor);
                                
                                // Auto-scroll
                                messagesContainer.scrollTop = messagesContainer.scrollHeight;
                            }
                        } catch (err) {
                            console.error('Error parsing SSE chunk:', err, dataStr);
                        }
                    }
                }
            }
            
            // Remove cursor when done
            if (assistantContentNode.cursor) {
                assistantContentNode.cursor.remove();
            }

        } catch (error) {
            console.error('Failed to stream response:', error);
            if (assistantContentNode.cursor) {
                assistantContentNode.cursor.remove();
            }
            const p = assistantContentNode.pElement;
            p.textContent = "Sorry, an error occurred while connecting to the Helix RPC Gateway.";
            p.style.color = "#ef4444";
        }
    });
});
