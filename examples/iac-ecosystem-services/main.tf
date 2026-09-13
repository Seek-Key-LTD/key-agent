terraform {
  required_version = ">= 1.5.0"
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

provider "docker" {
  host = var.docker_host
}

# ==============================================================================
# 1. Overleaf Community Edition (LaTeX 论文工场)
# ==============================================================================
resource "docker_container" "overleaf" {
  count   = var.enable_overleaf ? 1 : 0
  name    = "overleaf"
  image   = var.overleaf_image
  restart = "unless-stopped"

  ports {
    internal = 80
    external = var.overleaf_port
  }

  env = [
    "OVERLEAF_APP_NAME=${var.overleaf_app_name}",
    "OVERLEAF_ADMIN_EMAIL=${var.admin_email}",
    "OVERLEAF_MONGO_URL=${var.overleaf_mongo_url}",
    "OVERLEAF_REDIS_HOST=${var.redis_host}",
    "OVERLEAF_REDIS_PORT=${var.redis_port}",
    "SITE_MAINTENANCE_FILE=/etc/overleaf/site_status",
    "OPTIMISE_PDF=true",
    "SKIP_TEX_LIVE_CHECK=true",
    "ALLOW_MONGO_ADMIN_CHECK_FAILURES=true",
    "ADMIN_PRIVILEGE_AVAILABLE=true",
    "NODE_ENV=production"
  ]

  volumes {
    host_path      = "${var.data_root}/overleaf/data"
    container_path = "/var/lib/overleaf"
    read_only      = false
  }
}

# ==============================================================================
# 2. Drupal 11 Headless CMS (深度出版 / In-Depth Publishing)
# ==============================================================================
resource "docker_container" "drupal" {
  count   = var.enable_drupal ? 1 : 0
  name    = "drupal11"
  image   = var.drupal_image
  restart = "unless-stopped"

  ports {
    internal = 80
    external = var.drupal_port
  }

  env = [
    "DRUPAL_DB_DRIVER=${var.drupal_db_driver}",
    "DRUPAL_DB_HOST=${var.drupal_db_host}",
    "DRUPAL_DB_PORT=${var.drupal_db_port}",
    "DRUPAL_DB_NAME=${var.drupal_db_name}",
    "DRUPAL_DB_USER=${var.drupal_db_user}",
    "DRUPAL_DB_PASSWORD=${var.drupal_db_password}",
    "DRUPAL_TRUSTED_HOST_PATTERNS=${var.drupal_trusted_hosts}"
  ]

  volumes {
    host_path      = "${var.data_root}/drupal/web/sites"
    container_path = "/opt/drupal/web/sites"
    read_only      = false
  }
  volumes {
    host_path      = "${var.data_root}/drupal/web/modules"
    container_path = "/opt/drupal/web/modules"
    read_only      = false
  }
}

# ==============================================================================
# 3. Mastodon (长毛象 / 分布式联邦社交与认知广场)
# ==============================================================================
resource "docker_container" "mastodon_web" {
  count   = var.enable_mastodon ? 1 : 0
  name    = "mastodon-web"
  image   = var.mastodon_image
  restart = "unless-stopped"
  command = ["bundle", "exec", "puma", "-C", "config/puma.rb"]

  ports {
    internal = 3000
    external = var.mastodon_web_port
  }

  env = [
    "LOCAL_DOMAIN=${var.mastodon_domain}",
    "REDIS_HOST=${var.redis_host}",
    "REDIS_PORT=${var.redis_port}",
    "DB_HOST=${var.postgres_host}",
    "DB_PORT=${var.postgres_port}",
    "DB_NAME=mastodon_production",
    "DB_USER=${var.postgres_user}",
    "DB_PASS=${var.postgres_password}",
    "ES_ENABLED=true",
    "ES_HOST=${var.elasticsearch_host}",
    "ES_PORT=${var.elasticsearch_port}",
    "SECRET_KEY_BASE=${var.mastodon_secret_key_base}",
    "OTP_SECRET=${var.mastodon_otp_secret}"
  ]

  volumes {
    host_path      = "${var.data_root}/mastodon/public/system"
    container_path = "/mastodon/public/system"
    read_only      = false
  }
}

resource "docker_container" "mastodon_sidekiq" {
  count   = var.enable_mastodon ? 1 : 0
  name    = "mastodon-sidekiq"
  image   = var.mastodon_image
  restart = "unless-stopped"
  command = ["bundle", "exec", "sidekiq"]

  env = [
    "LOCAL_DOMAIN=${var.mastodon_domain}",
    "REDIS_HOST=${var.redis_host}",
    "REDIS_PORT=${var.redis_port}",
    "DB_HOST=${var.postgres_host}",
    "DB_PORT=${var.postgres_port}",
    "DB_NAME=mastodon_production",
    "DB_USER=${var.postgres_user}",
    "DB_PASS=${var.postgres_password}",
    "ES_ENABLED=true",
    "ES_HOST=${var.elasticsearch_host}",
    "ES_PORT=${var.elasticsearch_port}",
    "SECRET_KEY_BASE=${var.mastodon_secret_key_base}",
    "OTP_SECRET=${var.mastodon_otp_secret}"
  ]

  volumes {
    host_path      = "${var.data_root}/mastodon/public/system"
    container_path = "/mastodon/public/system"
    read_only      = false
  }
}

# ==============================================================================
# 4. RSSHub & FreshRSS (情报雷达与全文捕获)
# ==============================================================================
resource "docker_container" "rsshub" {
  count   = var.enable_rss_pipeline ? 1 : 0
  name    = "rsshub"
  image   = "diygod/rsshub:latest"
  restart = "unless-stopped"

  ports {
    internal = 1200
    external = 1200
  }

  env = [
    "NODE_ENV=production",
    "CACHE_TYPE=redis",
    "REDIS_URL=redis://${var.redis_host}:${var.redis_port}"
  ]
}

resource "docker_container" "freshrss" {
  count   = var.enable_rss_pipeline ? 1 : 0
  name    = "freshrss"
  image   = "freshrss/freshrss:latest"
  restart = "unless-stopped"

  ports {
    internal = 80
    external = 8088
  }

  env = [
    "TZ=Asia/Shanghai",
    "CRON_MIN=*/20"
  ]

  volumes {
    host_path      = "${var.data_root}/freshrss/data"
    container_path = "/var/www/FreshRSS/data"
    read_only      = false
  }
}
