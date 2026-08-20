# Import the whole-bucket rule by "bucket_id", or a prefixed rule by
# "bucket_id/prefix" — the prefix may itself contain slashes; everything after
# the first "/" is the prefix.
terraform import fpcloud_bucket_lifecycle_rule.all 8e2d5a7c-3f1b-4c9e-a6d8-0b7f5e3c1a9d
terraform import fpcloud_bucket_lifecycle_rule.logs 8e2d5a7c-3f1b-4c9e-a6d8-0b7f5e3c1a9d/logs/archive/
