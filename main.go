package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Subscriber struct {
	ID      string
	Closed  bool
	Channel chan []byte
}

type Event struct {
	Subscribers []Subscriber
}

func (e *Event) Subscribe() Subscriber {
	subscriber := Subscriber{
		ID:      fmt.Sprintf("%d", time.Now().Unix()),
		Channel: make(chan []byte),
	}
	e.Subscribers = append(e.Subscribers, subscriber)
	return subscriber
}

func (e *Event) Unsubscribe(ID string) {
	for i, s := range e.Subscribers {
		if s.ID != ID {
			continue
		}

		e.Subscribers = append(e.Subscribers[:i], e.Subscribers[i+1:]...)
		close(s.Channel)
		break
	}
}

func (e *Event) Publish(data []byte) {
	for _, subscriber := range e.Subscribers {
		subscriber.Channel <- data
	}
}

func receiveChatHandler(chatEvent *Event) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		subscriber := chatEvent.Subscribe()
		defer chatEvent.Unsubscribe(subscriber.ID)

		for {
			select {
			case data := <-subscriber.Channel:
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			case <-r.Context().Done():
				log.Println("Client disconnected")
				return
			}
		}
	}
}

type Chat struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

func sendChatHandler(chatEvent *Event) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		chat := Chat{}

		err := json.NewDecoder(r.Body).Decode(&chat)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		chatRaw, err := json.Marshal(chat)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		chatEvent.Publish(chatRaw)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Message sent"))
		return
	}
}

func htmlHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Chat with SSE</title>
</head>
<body>
  <h1>Server-Sent Events Chat</h1>

  <ul id="events"></ul>

  <form id="chat-form">
    <input type="text" id="user-id" placeholder="Enter your user ID" required>
    <input type="text" id="message" placeholder="Enter your message" required>
    <button type="submit">Send</button>
  </form>

  <script>
    const eventList = document.getElementById("events");
    const chatForm = document.getElementById("chat-form");
    const userIdInput = document.getElementById("user-id");
    const messageInput = document.getElementById("message");

    // Connect to the SSE endpoint.
    const evtSource = new EventSource("/chat/events");

    evtSource.onmessage = function(e) {
      const data = JSON.parse(e.data);
      const li = document.createElement("li");
      li.textContent = "Message from: " + data.user_id + " - " + data.message;
      eventList.appendChild(li);
    };

    evtSource.onerror = function(e) {
      console.error("Error:", e);
    };

    // Handle form submission
    chatForm.addEventListener("submit", async function(event) {
      event.preventDefault();

      const userId = userIdInput.value.trim();
      const message = messageInput.value.trim();

      if (!userId || !message) {
        alert("Both user ID and message are required!");
        return;
      }

      const payload = { user_id: userId, message: message };

      try {
        const response = await fetch("/chat/send", {
          method: "POST",
          headers: {
            "Content-Type": "application/json"
          },
          body: JSON.stringify(payload)
        });

        if (response.ok) {
          messageInput.value = ""; // Clear message input after sending
        } else {
          console.error("Failed to send message");
        }
      } catch (error) {
        console.error("Error sending message:", error);
      }
    });
  </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func main() {
	chatEvent := &Event{}

	http.HandleFunc("/chat/send", sendChatHandler(chatEvent))
	http.HandleFunc("/chat/events", receiveChatHandler(chatEvent))
	http.HandleFunc("/", htmlHandler)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
