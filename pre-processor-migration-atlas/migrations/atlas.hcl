# Atlas configuration for pre-processor-db migrations

variable "database_url" {
  type    = string
  default = getenv("DATABASE_URL")
}

env "local" {
  src = "file://schema.hcl"
  url = "postgres://pp_user:pp_db_pass_DO_NOT_USE_THIS@localhost:5437/pre_processor?sslmode=disable"
  dev = "docker://postgres/18/dev?search_path=public"

  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "kubernetes" {
  src = "file://schema.hcl"
  url = var.database_url

  migration {
    dir = "file://migrations"
  }

  diff {
    concurrent_index {
      create = false
      drop   = false
    }
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }

  lint {
    destructive {
      error = true
    }
    data_depend {
      error = true
    }
  }
}

env "ci" {
  src = "file://schema.hcl"
  dev = "docker://postgres/18/dev?search_path=public"

  migration {
    dir = "file://migrations"
  }

  lint {
    destructive {
      error = true
    }
    data_depend {
      error = true
    }
    naming {
      match   = "^[a-z][a-z0-9_]*$"
      message = "must be lowercase with underscores"
      error   = true
    }
    # MF101: Missing foreign key indexes
    MF101 {
      error = true
    }
  }
}

exec {
  schema = ["public"]
}
