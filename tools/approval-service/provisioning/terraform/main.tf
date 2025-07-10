
module "aws" {
    source = "./aws"
}

module "teleport" {
    source = "./teleport"
}

output "join_token" {
    value = module.teleport.bot_token
    sensitive = true
}
