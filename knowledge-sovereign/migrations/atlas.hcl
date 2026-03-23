# Atlas configuration for knowledge-sovereign-db migrations
# Pattern: recap-migration-atlas/migrations/atlas.hcl

variable "database_url" {
  type    = string
  default = getenv("DATABASE_URL")
}

env "local" {
  url = "postgres://alt:password@localhost:5434/knowledge_sovereign?sslmode=disable"
  dev = "docker://postgres/18/dev?search_path=public"

  migration {
    dir = "file://."
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "kubernetes" {
  url = var.database_url

  migration {
    dir = "file://."
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
  dev = "docker://postgres/18/dev?search_path=public"

  migration {
    dir = "file://."
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
