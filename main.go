package main

import (
	"fmt"
	"html/template"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/joho/godotenv"
	"github.com/nfnt/resize"
	"golang.org/x/crypto/bcrypt"
)

type Post struct {
	ID          uint      `gorm:"primary_key" json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url"`
	IsPublished bool      `json:"is_published"`
	Likes       int       `json:"likes"`
	Comments    int       `json:"comments"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Admin struct {
	ID       uint   `gorm:"primary_key"`
	Username string `gorm:"unique"`
	Password string
}

var db *gorm.DB

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	// Initialize database
	initDB()

	// Create default admin if not exists
	createDefaultAdmin()

	// Setup router
	r := gin.Default()

	// Load HTML templates with custom functions
	r.SetFuncMap(template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006 at 3:04 PM")
		},
		"timeAgo": func(t time.Time) string {
			duration := time.Since(t)
			if duration.Hours() > 24 {
				days := int(duration.Hours() / 24)
				return strconv.Itoa(days) + " days ago"
			} else if duration.Hours() >= 1 {
				hours := int(duration.Hours())
				return strconv.Itoa(hours) + " hours ago"
			} else if duration.Minutes() >= 1 {
				minutes := int(duration.Minutes())
				return strconv.Itoa(minutes) + " minutes ago"
			}
			return "Just now"
		},
		"subtract": func(a, b int) int {
			return a - b
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"mod": func(a, b int) int {
			return a % b
		},
		"gt": func(a, b int) bool {
			return a > b
		},
		"eq": func(a, b int) bool {
			return a == b
		},
	})

	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")

	// Public routes
	r.GET("/", homePage)
	r.GET("/post/:id", postDetail)
	r.GET("/privacy-policy", privacyPolicy)
	r.GET("/terms-of-service", termsOfService)
	r.GET("/robots.txt", robotsTxt)
	r.GET("/sitemap.xml", sitemapXML)

	// API routes
	api := r.Group("/api")
	{
		api.POST("/like/:id", likePost)
		api.POST("/upload", uploadImage)
	}

	// Admin routes
	admin := r.Group("/admin")
	admin.Use(adminAuth())
	{
		admin.GET("/", adminDashboard)
		admin.GET("/login", adminLogin)
		admin.POST("/login", adminLoginPost)
		admin.GET("/logout", adminLogout)
		admin.GET("/posts", adminPosts)
		admin.GET("/posts/new", adminNewPost)
		admin.POST("/posts", adminCreatePost)
		admin.GET("/posts/:id/edit", adminEditPost)
		admin.PUT("/posts/:id", adminUpdatePost)
		admin.DELETE("/posts/:id", adminDeletePost)
	}

	log.Println("Server starting on :2025")
	r.Run(":2025")
}

func initDB() {
	var err error
	db, err = gorm.Open("sqlite3", "feeds.db")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate
	db.AutoMigrate(&Post{}, &Admin{})
}

func createDefaultAdmin() {
	var admin Admin
	if db.Where("username = ?", "admin").First(&admin).RecordNotFound() {
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		defaultAdmin := Admin{
			Username: "admin",
			Password: string(hashedPassword),
		}
		db.Create(&defaultAdmin)
		log.Println("Default admin created: username=admin, password=admin123")
	}
}

// Public handlers
func homePage(c *gin.Context) {
	var posts []Post
	db.Where("is_published = ?", true).Order("created_at desc").Find(&posts)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title": "Num Guru - Numerology & Spiritual Guidance",
		"posts": posts,
	})
}

func postDetail(c *gin.Context) {
	id := c.Param("id")
	var post Post
	if db.First(&post, id).RecordNotFound() {
		c.HTML(http.StatusNotFound, "404.html", gin.H{
			"title": "Post Not Found",
		})
		return
	}

	c.HTML(http.StatusOK, "post-detail.html", gin.H{
		"title": post.Title + " - Num Guru",
		"post":  post,
	})
}

func privacyPolicy(c *gin.Context) {
	c.HTML(http.StatusOK, "privacy-policy.html", gin.H{
		"title": "Privacy Policy - Num Guru",
	})
}

func termsOfService(c *gin.Context) {
	c.HTML(http.StatusOK, "terms-of-service.html", gin.H{
		"title": "Terms of Service - Num Guru",
	})
}

func robotsTxt(c *gin.Context) {
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, `User-agent: *
Allow: /

Sitemap: https://numguru.org/sitemap.xml`)
}

func sitemapXML(c *gin.Context) {
	var posts []Post
	db.Where("is_published = ?", true).Find(&posts)

	c.Header("Content-Type", "application/xml")
	sitemap := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://numguru.org/</loc>
    <changefreq>daily</changefreq>
    <priority>1.0</priority>
  </url>`

	for _, post := range posts {
		sitemap += `
  <url>
    <loc>https://numguru.org/post/` + strconv.Itoa(int(post.ID)) + `</loc>
    <lastmod>` + post.UpdatedAt.Format("2006-01-02") + `</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>`
	}

	sitemap += `
</urlset>`

	c.String(http.StatusOK, sitemap)
}

// API handlers
func likePost(c *gin.Context) {
	id := c.Param("id")
	var post Post
	if db.First(&post, id).RecordNotFound() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	post.Likes++
	db.Save(&post)

	c.JSON(http.StatusOK, gin.H{"likes": post.Likes})
}

// Admin middleware
func adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/admin/login" {
			c.Next()
			return
		}

		session, _ := c.Cookie("admin_session")
		if session != "authenticated" {
			c.Redirect(http.StatusFound, "/admin/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// Admin handlers
func adminDashboard(c *gin.Context) {
	var postCount int
	var publishedCount int

	db.Model(&Post{}).Count(&postCount)
	db.Model(&Post{}).Where("is_published = ?", true).Count(&publishedCount)

	c.HTML(http.StatusOK, "admin-dashboard.html", gin.H{
		"title":          "Admin Dashboard - Num Guru",
		"postCount":      postCount,
		"publishedCount": publishedCount,
	})
}

func adminLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "admin-login.html", gin.H{
		"title": "Admin Login - Num Guru",
	})
}

func adminLoginPost(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	var admin Admin
	if db.Where("username = ?", username).First(&admin).RecordNotFound() {
		c.HTML(http.StatusOK, "admin-login.html", gin.H{
			"title": "Admin Login - Num Guru",
			"error": "Invalid credentials",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(password)); err != nil {
		c.HTML(http.StatusOK, "admin-login.html", gin.H{
			"title": "Admin Login - Num Guru",
			"error": "Invalid credentials",
		})
		return
	}

	c.SetCookie("admin_session", "authenticated", 3600*24, "/", "", false, true)
	c.Redirect(http.StatusFound, "/admin/")
}

func adminLogout(c *gin.Context) {
	c.SetCookie("admin_session", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/admin/login")
}

func adminPosts(c *gin.Context) {
	var posts []Post
	db.Order("created_at desc").Find(&posts)

	c.HTML(http.StatusOK, "admin-posts.html", gin.H{
		"title": "Manage Posts - Num Guru",
		"posts": posts,
	})
}

func adminNewPost(c *gin.Context) {
	c.HTML(http.StatusOK, "admin-post-form.html", gin.H{
		"title": "New Post - Num Guru",
	})
}

func adminCreatePost(c *gin.Context) {
	post := Post{
		Title:       c.PostForm("title"),
		Content:     c.PostForm("content"),
		Author:      c.PostForm("author"),
		ImageURL:    c.PostForm("image_url"),
		IsPublished: c.PostForm("is_published") == "on",
	}

	if err := db.Create(&post).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/admin/posts")
}

func adminEditPost(c *gin.Context) {
	id := c.Param("id")
	var post Post
	if db.First(&post, id).RecordNotFound() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	c.HTML(http.StatusOK, "admin-post-form.html", gin.H{
		"title": "Edit Post - Num Guru",
		"post":  post,
	})
}

func adminUpdatePost(c *gin.Context) {
	id := c.Param("id")
	var post Post
	if db.First(&post, id).RecordNotFound() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	post.Title = c.PostForm("title")
	post.Content = c.PostForm("content")
	post.Author = c.PostForm("author")
	post.ImageURL = c.PostForm("image_url")
	post.IsPublished = c.PostForm("is_published") == "on"

	if err := db.Save(&post).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Redirect(http.StatusFound, "/admin/posts")
}

func adminDeletePost(c *gin.Context) {
	id := c.Param("id")
	if err := db.Delete(&Post{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Post deleted successfully"})
}

// Helper functions
func isAuthenticated(c *gin.Context) bool {
	session, err := c.Cookie("admin_session")
	return err == nil && session == "authenticated"
}

// Image upload functions
func uploadImage(c *gin.Context) {
	if !isAuthenticated(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Validate file type
	if !isValidImageType(header.Filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Only JPG, JPEG, PNG are allowed"})
		return
	}

	// Validate file size (max 5MB)
	if header.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum size is 5MB"})
		return
	}

	// Generate unique filename
	filename := generateImageFilename(header.Filename)
	uploadPath := filepath.Join("static", "uploads", "posts", filename)

	// Create upload directory if it doesn't exist
	os.MkdirAll(filepath.Dir(uploadPath), 0755)

	// Save original image
	if err := saveUploadedFile(file, uploadPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Create thumbnails
	createImageThumbnails(uploadPath, filename)

	// Return the image URL
	imageURL := "/" + strings.Replace(uploadPath, "\\", "/", -1)
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"imageUrl": imageURL,
		"filename": filename,
	})
}

func isValidImageType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png"
}

func generateImageFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)
	uuid := uuid.New().String()
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s%s", timestamp, uuid[:8], ext)
}

func saveUploadedFile(file multipart.File, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	return err
}

func createImageThumbnails(originalPath, filename string) {
	// Open original image
	file, err := os.Open(originalPath)
	if err != nil {
		log.Printf("Error opening image for thumbnail creation: %v", err)
		return
	}
	defer file.Close()

	// Decode image
	img, format, err := image.Decode(file)
	if err != nil {
		log.Printf("Error decoding image: %v", err)
		return
	}

	// Create medium size (800px width)
	mediumImg := resize.Resize(800, 0, img, resize.Lanczos3)
	mediumPath := strings.Replace(originalPath, filepath.Ext(originalPath), "_medium"+filepath.Ext(originalPath), 1)
	saveThumbnail(mediumImg, mediumPath, format)

	// Create thumbnail size (300px width)
	thumbImg := resize.Resize(300, 0, img, resize.Lanczos3)
	thumbPath := strings.Replace(originalPath, filepath.Ext(originalPath), "_thumb"+filepath.Ext(originalPath), 1)
	saveThumbnail(thumbImg, thumbPath, format)
}

func saveThumbnail(img image.Image, path, format string) {
	out, err := os.Create(path)
	if err != nil {
		log.Printf("Error creating thumbnail file: %v", err)
		return
	}
	defer out.Close()

	switch format {
	case "jpeg":
		jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	case "png":
		png.Encode(out, img)
	}
}
