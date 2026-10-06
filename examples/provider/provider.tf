terraform {
  required_providers {
    optimizelycmssaas = {
      source = "willemharingopti/optimizelycmssaas" # replace with your registry address
    }
  }
}

# Credentials are read from the environment (recommended; keeps the secret out of config and state):
#   export OPTIMIZELY_CMS_CLIENT_ID=...        # the API client's NAME
#   export OPTIMIZELY_CMS_CLIENT_SECRET=...
#   export OPTIMIZELY_CMS_BASE_URL=https://api.cms.optimizely.com   # optional
provider "optimizelycmssaas" {}
