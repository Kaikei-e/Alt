# Atlas configuration for recap-db migrations
# Updated to Atlas v0.31+ best practices

variable "database_url" {
  type    = string
  default = getenv("DATABASE_URL")
}

env "local" {
  src = "file://schema.hcl"
  url = "postgres://recap_user:recap_db_pass_DO_NOT_USE_THIS@localhost:5435/recap?sslmode=disable"
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
    dir      = "file://migrations"
    baseline = "20240101000300"
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

env "default" {
  for_each = toset(["local", "kubernetes"])
  url      = atlas.env[each.key].url
  src      = atlas.env[each.key].src

  migration {
    dir = atlas.env[each.key].migration.dir
  }
}

exec {
  schema = ["public"]
}

format {
  migrate {
    apply = format(
      "-- Migration: %s\n-- Created: %s\n-- Atlas Version: %s\n\n%s",
      "{{ .Name }}",
      "{{ .Time }}",
      "{{ .Version }}",
      "{{ sql . \"  \" }}"
    )
  }
}
