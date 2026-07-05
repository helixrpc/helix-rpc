document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('chat-form');
  const input = document.getElementById('chat-input');
  const messagesContainer = document.getElementById('messages');
  const sendBtn = document.getElementById('send-btn');
  const ttftDisplay = document.getElementById('ttft');
  const speedDisplay = document.getElementById('tokens-sec');

  // Auto-resize textarea
  input.addEventListener('input', function() {
    this.style.height = 'auto';
    this.style.height = (this.scrollHeight) + 'px';
    if(this.value.trim() === '') {
      sendBtn.disabled = true;
    } else {
      sendBtn.disabled = false;
    }
  });

  // Handle enter key to submit
  input.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if(!sendBtn.disabled) {
        form.dispatchEvent(new Event('submit'));
      }
    }
  });

  function appendMessage(role, text, isStreaming = false) {
    const msgDiv = document.createElement('div');
    msgDiv.className = `message ${role}`;
    
    const avatarDiv = document.createElement('div');
    avatarDiv.className = 'avatar';
    avatarDiv.textContent = role === 'user' ? 'U' : '⚡';
    
    const contentDiv = document.createElement('div');
    contentDiv.className = 'content';
    
    const p = document.createElement('p');
    p.textContent = text;
    contentDiv.appendChild(p);
    
    let cursor = null;
    if (isStreaming) {
      cursor = document.createElement('span');
      cursor.className = 'cursor';
      contentDiv.appendChild(cursor);
    }
    
    msgDiv.appendChild(avatarDiv);
    msgDiv.appendChild(contentDiv);
    messagesContainer.appendChild(msgDiv);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
    
    return { pElement: p, cursor: cursor, msgDiv: msgDiv };
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    
    const prompt = input.value.trim();
    if (!prompt) return;

    // Reset input
    input.value = '';
    input.style.height = 'auto';
    sendBtn.disabled = true;
    input.disabled = true;
    
    appendMessage('user', prompt);
    const { pElement, cursor } = appendMessage('assistant', '', true);
    
    // Stats tracking
    let startTime = performance.now();
    let firstTokenTime = 0;
    let tokenCount = 0;
    
    ttftDisplay.textContent = 'waiting...';
    speedDisplay.textContent = 'calc...';
    
    try {
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
        
        buffer = lines.pop(); // keep incomplete line

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const dataStr = line.slice(6).trim();
            if (dataStr === '[DONE]') {
              break;
            }
            
            try {
              const data = JSON.parse(dataStr);
              if (data.choices && data.choices[0].delta && data.choices[0].delta.content) {
                const textChunk = data.choices[0].delta.content;
                
                // Track stats
                if (tokenCount === 0) {
                  firstTokenTime = performance.now();
                  ttftDisplay.textContent = `${(firstTokenTime - startTime).toFixed(0)} ms`;
                }
                tokenCount++;
                
                // Update speed
                if (tokenCount > 1) {
                  const elapsedSeconds = (performance.now() - firstTokenTime) / 1000;
                  const tps = (tokenCount / elapsedSeconds).toFixed(1);
                  speedDisplay.textContent = `${tps} t/s`;
                }

                // Append text
                pElement.textContent += textChunk;
                messagesContainer.scrollTop = messagesContainer.scrollHeight;
              }
            } catch (err) {
              console.error('Error parsing SSE chunk:', err, dataStr);
            }
          }
        }
      }
      
    } catch (error) {
      console.error('Failed to stream response:', error);
      pElement.textContent = "Sorry, an error occurred while connecting to the server.";
      pElement.style.color = "#ef4444";
    } finally {
      if (cursor) cursor.remove();
      sendBtn.disabled = false;
      input.disabled = false;
      input.focus();
    }
  });
});
