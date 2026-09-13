variable "docker_host" {
  type        = string
  description = "Docker daemon socket or Podman socket (e.g. unix:///var/run/docker.sock or unix:///run/podman/podman.sock)"
  default     = "unix:///var/run/docker.sock"
}

variable "data_root" {
  type        = string
  description = "Root directory for persistent container volume mounts"
  default     = "/opt/asn-data"
}

variable "admin_email" {
  type        = string
  description = "System administrator email"
  default     = "admin@example.com"
}

# --- Redis & Postgres & ES ---
variable "redis_host" {
  type    = string
  default = "127.0.0.1"
}

variable "redis_port" {
  type    = number
  default = 6379
}

variable "postgres_host" {
  type    = string
  default = "127.0.0.1"
}

variable "postgres_port" {
  type    = number
  default = 5432
}

variable "postgres_user" {
  type    = string
  default = "postgres"
}

variable "postgres_password" {
  type      = string
  sensitive = true
  default   = "change_me_postgres"
}

variable "elasticsearch_host" {
  type    = string
  default = "127.0.0.1"
}

variable "elasticsearch_port" {
  type    = number
  default = 9200
}

# --- Overleaf ---
variable "enable_overleaf" {
  type    = bool
  default = true
}

variable "overleaf_image" {
  type    = string
  default = "sharelatex/sharelatex:latest"
}

variable "overleaf_port" {
  type    = number
  default = 8080
}

variable "overleaf_app_name" {
  type    = string
  default = "SeekKey Overleaf Factory"
}

variable "overleaf_mongo_url" {
  type        = string
  sensitive   = true
  description = "MongoDB connection string (Local or MongoDB Atlas 512MB free tier)"
  default     = "mongodb://127.0.0.1:27017/overleaf"
}

# --- Drupal ---
variable "enable_drupal" {
  type    = bool
  default = true
}

variable "drupal_image" {
  type    = string
  default = "drupal:11-apache"
}

variable "drupal_port" {
  type    = number
  default = 8081
}

variable "drupal_db_driver" {
  type    = string
  default = "pgsql"
}

variable "drupal_db_host" {
  type    = string
  default = "127.0.0.1"
}

variable "drupal_db_port" {
  type    = number
  default = 5432
}

variable "drupal_db_name" {
  type    = string
  default = "drupal"
}

variable "drupal_db_user" {
  type    = string
  default = "drupal"
}

variable "drupal_db_password" {
  type      = string
  sensitive = true
  default   = "change_me_drupal"
}

variable "drupal_trusted_hosts" {
  type    = string
  default = "^.*$"
}

# --- Mastodon ---
variable "enable_mastodon" {
  type    = bool
  default = true
}

variable "mastodon_image" {
  type    = string
  default = "tootsuite/mastodon:v4.3.0"
}

variable "mastodon_domain" {
  type    = string
  default = "social.example.com"
}

variable "mastodon_web_port" {
  type    = number
  default = 3000
}

variable "mastodon_secret_key_base" {
  type      = string
  sensitive = true
  default   = "generate_with_rake_secret_64char"
}

variable "mastodon_otp_secret" {
  type      = string
  sensitive = true
  default   = "generate_with_rake_secret_64char"
}

# --- RSS ---
variable "enable_rss_pipeline" {
  type    = bool
  default = true
}
