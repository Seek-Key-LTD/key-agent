output "overleaf_endpoint" {
  value = var.enable_overleaf ? "http://${var.docker_host == "unix:///var/run/docker.sock" ? "localhost" : "remote"}:${var.overleaf_port}" : "disabled"
}

output "drupal_endpoint" {
  value = var.enable_drupal ? "http://${var.docker_host == "unix:///var/run/docker.sock" ? "localhost" : "remote"}:${var.drupal_port}" : "disabled"
}

output "mastodon_endpoint" {
  value = var.enable_mastodon ? "http://${var.docker_host == "unix:///var/run/docker.sock" ? "localhost" : "remote"}:${var.mastodon_web_port}" : "disabled"
}

output "freshrss_endpoint" {
  value = var.enable_rss_pipeline ? "http://${var.docker_host == "unix:///var/run/docker.sock" ? "localhost" : "remote"}:8088" : "disabled"
}
