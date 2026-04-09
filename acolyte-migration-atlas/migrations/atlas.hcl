# Atlas configuration for acolyte-db migrations

variable "database_url" {
  type    = string
  default = getenv("DATABASE_URL")
}

env "local" {
  src = "file://schema.hcl"
  url = "postgres://acolyte_user:password@localhost:5439/acolyte?sslmode=disable"
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

  lint {
    destructive {
      error = true
    }
    data_depend {
      error = true
    }
  }
}
