package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	//"flag"

	// "math/rand"
	// "runtime"
	"sort"

	"time"

	"github.com/asynkron/protoactor-go/actor"
)

type CreateUserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}
// Configuration
type Config struct {
	MaxUsersPerSubreddit int
	MaxPostsPerUser      int
	MaxCommentsPerPost   int
	KarmaMultiplier      float64
	SimulationSpeed      float64
	NetworkLatency       time.Duration
	DisconnectionRate    float64
}

// Default configuration
var DefaultConfig = &Config{
	MaxUsersPerSubreddit: 1000000,
	MaxPostsPerUser:      100,
	MaxCommentsPerPost:   1000,
	KarmaMultiplier:      1.0,
	SimulationSpeed:      1.0,
	NetworkLatency:       50 * time.Millisecond,
	DisconnectionRate:    0.1,
}

// Core domain models
type User struct {
	ID         string
	Username   string
	Karma      int
	Created    time.Time
	PID        *actor.PID
	Subreddits map[string]bool
	Feed       []*Post
}

type Subreddit struct {
	ID          string
	Name        string
	Description string
	Members     map[string]bool
	Posts       []*Post
	Created     time.Time
}

type Post struct {
	ID          string
	Title       string
	Content     string
	AuthorID    string
	SubredditID string
	Upvotes     int
	Downvotes   int
	Comments    []*Comment
	Created     time.Time
}

type Comment struct {
	ID        string
	Content   string
	AuthorID  string
	PostID    string
	ParentID  string
	Upvotes   int
	Downvotes int
	Created   time.Time
	Children  []*Comment
}

type DirectMessage struct {
	ID         string
	FromUserID string
	ToUserID   string
	Content    string
	Created    time.Time
	IsRead     bool
}

// DetailedMetrics struct to store simulation metrics
type DetailedMetrics struct {
	StartTime           time.Time
	TotalUsers          int
	TotalSubreddits     int
	TotalPosts          int
	TotalComments       int
	ActiveUsers         int
	TotalKarma          int
	AverageKarmaPerUser float64
	SimulationDuration  time.Duration
	ActionsPerSecond    float64
	MemoryUsage         uint64
}

// Reddit Engine
type RedditEngine struct {
	mu             sync.RWMutex
	system         *actor.ActorSystem
	users          map[string]*User
	subreddits     map[string]*Subreddit
	directMessages map[string][]*DirectMessage
	metrics        *DetailedMetrics
	config         *Config
}

func NewRedditEngine(config *Config) *RedditEngine {
	return &RedditEngine{
		system:         actor.NewActorSystem(),
		users:          make(map[string]*User),
		subreddits:     make(map[string]*Subreddit),
		directMessages: make(map[string][]*DirectMessage),
		metrics:        &DetailedMetrics{StartTime: time.Now()},
		config:         config,
	}
}

// User Creation
func (e *RedditEngine) CreateUser(username string) (*CreateUserResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, user := range e.users {
		if user.Username == username {
			return nil, fmt.Errorf("username already exists")
		}
	}

	userID := fmt.Sprintf("user_%d", len(e.users))
	newUser := &User{
		ID:         userID,
		Username:   username,
		Created:    time.Now(),
		Subreddits: make(map[string]bool),
		Feed:       make([]*Post, 0),
		Karma:      0,
	}
	e.users[userID] = newUser
	e.metrics.TotalUsers++
	return &CreateUserResponse{
		ID:       newUser.ID,
		Username: newUser.Username,
	}, nil
}

// Subreddit Creation
func (e *RedditEngine) CreateSubreddit(name, description string) (*Subreddit, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, subreddit := range e.subreddits {
		if subreddit.Name == name {
			return nil, fmt.Errorf("subreddit already exists")
		}
	}

	subredditID := fmt.Sprintf("subreddit_%d", len(e.subreddits))
	newSubreddit := &Subreddit{
		ID:          subredditID,
		Name:        name,
		Description: description,
		Members:     make(map[string]bool),
		Posts:       make([]*Post, 0),
		Created:     time.Now(),
	}
	e.subreddits[subredditID] = newSubreddit
	e.metrics.TotalSubreddits++
	return newSubreddit, nil
}

// Post Creation
func (e *RedditEngine) CreatePost(authorID, subredditID, title, content string) (*Post, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	user, userExists := e.users[authorID]
	subreddit, subredditExists := e.subreddits[subredditID]
	if !userExists || !subredditExists {
		return nil, fmt.Errorf("invalid user or subreddit")
	}

	postID := fmt.Sprintf("post_%d", len(subreddit.Posts))
	newPost := &Post{
		ID:          postID,
		Title:       title,
		Content:     content,
		AuthorID:    authorID,
		SubredditID: subredditID,
		Created:     time.Now(),
		Comments:    make([]*Comment, 0),
	}

	subreddit.Posts = append(subreddit.Posts, newPost)
	user.Feed = append(user.Feed, newPost)
	e.metrics.TotalPosts++
	return newPost, nil
}

// Comment Creation
func (e *RedditEngine) CreateComment(authorID, postID, parentID, content string) (*Comment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var targetPost *Post
	for _, subreddit := range e.subreddits {
		for _, post := range subreddit.Posts {
			if post.ID == postID {
				targetPost = post
				break
			}
		}
		if targetPost != nil {
			break
		}
	}
	if targetPost == nil {
		return nil, fmt.Errorf("post not found")
	}

	commentID := fmt.Sprintf("comment_%d", len(targetPost.Comments))
	newComment := &Comment{
		ID:       commentID,
		Content:  content,
		AuthorID: authorID,
		PostID:   postID,
		ParentID: parentID,
		Created:  time.Now(),
		Children: make([]*Comment, 0),
	}

	if parentID == "" {
		targetPost.Comments = append(targetPost.Comments, newComment)
	} else {
		parentComment := e.findComment(targetPost.Comments, parentID)
		if parentComment == nil {
			return nil, fmt.Errorf("parent comment not found")
		}
		parentComment.Children = append(parentComment.Children, newComment)
	}

	e.metrics.TotalComments++
	return newComment, nil
}

func (e *RedditEngine) findComment(comments []*Comment, commentID string) *Comment {
	for _, comment := range comments {
		if comment.ID == commentID {
			return comment
		}
		if found := e.findComment(comment.Children, commentID); found != nil {
			return found
		}
	}
	return nil
}

// Voting Mechanism
func (e *RedditEngine) Vote(userID, itemID string, isUpvote bool, itemType string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch itemType { // case for posts and comments
	case "post":
		for _, subreddit := range e.subreddits {
			for _, post := range subreddit.Posts {
				if post.ID == itemID {
					if isUpvote {
						post.Upvotes++
					} else {
						post.Downvotes++
					}
					if user, exists := e.users[userID]; exists {
						user.Karma += map[bool]int{true: 1, false: -1}[isUpvote]
					}
					return nil
				}
			}
		}
	case "comment":
		for _, subreddit := range e.subreddits {
			for _, post := range subreddit.Posts {
				if comment := e.findComment(post.Comments, itemID); comment != nil {
					if isUpvote {
						comment.Upvotes++
					} else {
						comment.Downvotes++
					}
					if user, exists := e.users[userID]; exists {
						user.Karma += map[bool]int{true: 1, false: -1}[isUpvote]
					}
					return nil
				}
			}
		}
	}
	return fmt.Errorf("item not found")
}

// Get User Feed
func (e *RedditEngine) GetUserFeed(userID string, limit int) ([]*Post, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	user, exists := e.users[userID]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	sort.Slice(user.Feed, func(i, j int) bool {
		return user.Feed[i].Created.After(user.Feed[j].Created)
	})

	if limit > 0 && len(user.Feed) > limit {
		return user.Feed[:limit], nil
	}
	return user.Feed, nil
}

// Join Subreddit
func (e *RedditEngine) JoinSubreddit(userID, subredditID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if the user and subreddit exist
	user, userExists := e.users[userID]
	subreddit, subredditExists := e.subreddits[subredditID]
	fmt.Print(!userExists)
	fmt.Print(!subredditExists)
	fmt.Print(e.users[userID])
	fmt.Print(e.subreddits[subredditID])

	if !userExists || !subredditExists {
		return fmt.Errorf("invalid user or subreddit")
	}

	// Check if the user is already a member of the subreddit
	if _, isMember := user.Subreddits[subredditID]; isMember {
		return fmt.Errorf("user '%s' is already a member of subreddit '%s'", userID, subredditID)
	}

	// Add the user to the subreddit
	user.Subreddits[subredditID] = true
	subreddit.Members[userID] = true
	return nil
}


// Leave Subreddit
func (e *RedditEngine) LeaveSubreddit(userID, subredditID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	user, userExists := e.users[userID]
	subreddit, subredditExists := e.subreddits[subredditID]

	if !userExists || !subredditExists {
		return fmt.Errorf("invalid user or subreddit")
	}

	delete(user.Subreddits, subredditID)
	delete(subreddit.Members, userID)
	return nil
}

// Send Direct Message
func (e *RedditEngine) SendDirectMessage(fromUserID, toUserID, content string) (*DirectMessage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, fromExists := e.users[fromUserID]
	_, toExists := e.users[toUserID]

	if !fromExists || !toExists {
		return nil, fmt.Errorf("invalid user(s)")
	}

	messageID := fmt.Sprintf("dm_%d", len(e.directMessages[toUserID]))
	newMessage := &DirectMessage{
		ID:         messageID,
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Content:    content,
		Created:    time.Now(),
		IsRead:     false,
	}

	e.directMessages[toUserID] = append(e.directMessages[toUserID], newMessage)
	return newMessage, nil
}

// Get Direct Messages
func (e *RedditEngine) GetDirectMessages(userID string) ([]*DirectMessage, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	_, exists := e.users[userID]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	return e.directMessages[userID], nil
}

// Reply to Direct Message
func (e *RedditEngine) ReplyDirectMessage(messageID, content, userID string) (*DirectMessage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var originalMessage *DirectMessage
	for _, messages := range e.directMessages {
		for _, msg := range messages {
			if msg.ID == messageID {
				originalMessage = msg
				break
			}
		}
		if originalMessage != nil {
			break
		}
	}
	//debug/error msgs
	if originalMessage == nil {
		return nil, fmt.Errorf("message not found")
	}

	if originalMessage.ToUserID != userID {
		return nil, fmt.Errorf("user not authorized to reply to this message")
	}

	replyMessage, err := e.SendDirectMessage(userID, originalMessage.FromUserID, content)
	if err != nil {
		return nil, err
	}

	return replyMessage, nil
}
// Define global variables for the engine and mutex
var (
	engine *RedditEngine
	mu     sync.RWMutex
)

// Initialize the Reddit engine
func init() {
	config := DefaultConfig
	engine = NewRedditEngine(config)
}

// Handler functions for the REST API
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.Username == "" {
		http.Error(w, "Missing username", http.StatusBadRequest)
		return
	}

	mu.Lock()
	user, err := engine.CreateUser(payload.Username)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(user)
}

func createSubredditHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.Name == "" || payload.Description == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	subreddit, err := engine.CreateSubreddit(payload.Name, payload.Description)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(subreddit)
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AuthorID    string `json:"authorID"`
		SubredditID string `json:"subredditID"`
		Title       string `json:"title"`
		Content     string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.AuthorID == "" || payload.SubredditID == "" || payload.Title == "" || payload.Content == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	post, err := engine.CreatePost(payload.AuthorID, payload.SubredditID, payload.Title, payload.Content)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(post)
}

func createCommentHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AuthorID string `json:"authorID"`
		PostID   string `json:"postID"`
		ParentID string `json:"parentID"`
		Content  string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.AuthorID == "" || payload.PostID == "" || payload.Content == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	comment, err := engine.CreateComment(payload.AuthorID, payload.PostID, payload.ParentID, payload.Content)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(comment)
}

func voteHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID   string `json:"userID"`
		ItemID   string `json:"itemID"`
		ItemType string `json:"itemType"`
		IsUpvote bool   `json:"isUpvote"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" || payload.ItemID == "" || payload.ItemType == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	err := engine.Vote(payload.UserID, payload.ItemID, payload.IsUpvote, payload.ItemType)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Write([]byte("Vote recorded"))
}

func sendDirectMessageHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		FromUserID string `json:"fromUserID"`
		ToUserID   string `json:"toUserID"`
		Content    string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.FromUserID == "" || payload.ToUserID == "" || payload.Content == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	message, err := engine.SendDirectMessage(payload.FromUserID, payload.ToUserID, payload.Content)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	json.NewEncoder(w).Encode(message)
}

func joinSubredditHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID      string `json:"userID"`
		SubredditID string `json:"subredditID"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.UserID == ""  {
		http.Error(w, "Missing user name", http.StatusBadRequest)
		return
	}

	if payload.SubredditID == "" {
		http.Error(w, "Missing subreddit", http.StatusBadRequest)
		return
	}
	mu.Lock()
	err := engine.JoinSubreddit(payload.UserID, payload.SubredditID)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Write([]byte("Joined subreddit"))
}

func leaveSubredditHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID      string `json:"userID"`
		SubredditID string `json:"subredditID"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" || payload.SubredditID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	mu.Lock()
	err := engine.LeaveSubreddit(payload.UserID, payload.SubredditID)
	mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Write([]byte("Left subreddit"))
}

func getUserFeedHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID string `json:"userID"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" {
		http.Error(w, "Missing Parameters", http.StatusBadRequest)
		return
	}

	mu.RLock()
	feed, err := engine.GetUserFeed(payload.UserID, -1)
	mu.RUnlock()

	if err != nil {
		http.Error(w, "Error fetching feed", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(feed)
}

func getDirectMessagesHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID string `json:"userID"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" {
		http.Error(w, "Missing Parameters", http.StatusBadRequest)
		return
	}

	mu.RLock()
	messages, err := engine.GetDirectMessages(payload.UserID)
	mu.RUnlock()

	if err != nil {
		http.Error(w, "Error fetching messages", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(messages)
}

// Main function to start the server
func main() {
	http.HandleFunc("/createUser", createUserHandler)
	http.HandleFunc("/createSubreddit", createSubredditHandler)
	http.HandleFunc("/createPost", createPostHandler)
	http.HandleFunc("/createComment", createCommentHandler)
	http.HandleFunc("/vote", voteHandler)
	http.HandleFunc("/sendDirectMessage", sendDirectMessageHandler)
	http.HandleFunc("/joinSubreddit", joinSubredditHandler)
	http.HandleFunc("/leaveSubreddit", leaveSubredditHandler)
	http.HandleFunc("/getUserFeed", getUserFeedHandler)
	http.HandleFunc("/getDirectMessages", getDirectMessagesHandler)

	fmt.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}