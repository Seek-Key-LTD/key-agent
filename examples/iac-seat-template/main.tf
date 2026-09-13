# =============================================================================
# ASN Agent Seat Provisioning Terraform Module Template
# =============================================================================

variable "agent_name" {
  type        = string
  description = "Agent 唯一标识符 (例如: putuo, zhuhu, yuyang)"
}

variable "soul_name" {
  type        = string
  description = "前世花名 (例如: 老林, 竺教授, 老于)"
}

variable "target_node" {
  type        = string
  default     = "nuc"
  description = "目标物理/云端部署节点 (如 ash3c, ch4, nuc, raccoon)"
}

variable "llm_model" {
  type        = string
  default     = "claude-3-7-sonnet"
  description = "默认挂接的基础推理大模型"
}

# 1. 自动生成专属安全随机密码
resource "random_password" "agent_secret" {
  length  = 32
  special = false
}

# 2. Authentik IDP 服务账号创世
resource "authentik_user" "agent_sa" {
  username = "agent-sa-${var.agent_name}"
  name     = "Agent Service Account - ${var.agent_name}"
  type     = "service_account"
}

# 3. Authentik M2M OAuth2 Provider
resource "authentik_provider_oauth2" "agent_oauth" {
  name          = "agent-provider-${var.agent_name}"
  client_type   = "confidential"
  client_id     = "agent-client-${var.agent_name}"
  client_secret = random_password.agent_secret.result
}

# 4. Authentik Application
resource "authentik_application" "agent_app" {
  name              = "Agent Node - ${var.agent_name}"
  slug              = "agent-${var.agent_name}"
  protocol_provider = authentik_provider_oauth2.agent_oauth.id
}

# 5. Vault 私有凭据自动注入
resource "vault_generic_secret" "agent_vault_creds" {
  path = "secret/dev/authentik/${var.agent_name}"
  data_json = jsonencode({
    AUTHENTIK_CLIENT_ID     = authentik_provider_oauth2.agent_oauth.client_id
    AUTHENTIK_CLIENT_SECRET = authentik_provider_oauth2.agent_oauth.client_secret
    SOUL_NAME               = var.soul_name
    LLM_MODEL               = var.llm_model
  })
}

output "agent_endpoint" {
  value = "https://keyagent-${var.target_node}.capitaltrain.cn/a2a"
}
