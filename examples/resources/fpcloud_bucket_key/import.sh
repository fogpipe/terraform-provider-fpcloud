# Import a bucket key by "bucket_id/access_key_id". The secret access key is not
# recoverable on import (it is returned only at creation).
terraform import fpcloud_bucket_key.readonly bkt-abc123/GK1234567890abcdef
