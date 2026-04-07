"""
SQLite -> PostgreSQL 数据迁移工具。

当前实现将 SQLite 源库中的已知 ORM 表按依赖顺序复制到目标数据库。
默认要求目标库为空；如需覆盖导入，请显式启用 replace_target。
"""

from __future__ import annotations

import argparse
import os
from typing import Any

from sqlalchemy import Integer, create_engine, func, inspect, select, text

from .models import Base
from . import team_models  # noqa: F401  # 确保 team_* 表注册到 metadata


def normalize_database_url(database_url: str) -> str:
    if database_url.startswith("postgres://"):
        return "postgresql+psycopg://" + database_url[len("postgres://"):]
    if database_url.startswith("postgresql://"):
        return "postgresql+psycopg://" + database_url[len("postgresql://"):]
    return database_url


def normalize_sqlite_url(database_url: str) -> str:
    if database_url.startswith("sqlite:///"):
        return database_url
    return f"sqlite:///{os.path.abspath(database_url)}"


def _count_rows(conn, table) -> int:
    return int(conn.execute(select(func.count()).select_from(table)).scalar_one())


def _sync_postgres_sequences(conn, tables) -> None:
    if conn.dialect.name != "postgresql":
        return

    for table in tables:
        primary_key_columns = list(table.primary_key.columns)
        if len(primary_key_columns) != 1:
            continue

        column = primary_key_columns[0]
        if not isinstance(column.type, Integer):
            continue

        table_name = table.name.replace('"', '""')
        column_name = column.name.replace('"', '""')
        conn.execute(
            text(
                f"""
                SELECT setval(
                    pg_get_serial_sequence('"{table_name}"', :column_name),
                    COALESCE((SELECT MAX("{column_name}") FROM "{table_name}"), 1),
                    (SELECT MAX("{column_name}") IS NOT NULL FROM "{table_name}")
                )
                """
            ),
            {"column_name": column.name},
        )


def migrate_sqlite_to_database(
    source_sqlite_url: str,
    target_database_url: str,
    *,
    replace_target: bool = False,
    batch_size: int = 500,
) -> dict[str, Any]:
    """将 SQLite 数据复制到目标数据库。"""

    if batch_size <= 0:
        raise ValueError("batch_size 必须大于 0")

    source_url = normalize_sqlite_url(source_sqlite_url)
    target_url = normalize_database_url(target_database_url)

    source_engine = create_engine(
        source_url,
        connect_args={"check_same_thread": False, "timeout": 30},
        pool_pre_ping=True,
    )
    target_engine = create_engine(target_url, pool_pre_ping=True)

    try:
        Base.metadata.create_all(bind=target_engine)

        source_table_names = set(inspect(source_engine).get_table_names())
        tables = [table for table in Base.metadata.sorted_tables if table.name in source_table_names]

        with source_engine.connect() as source_conn, target_engine.begin() as target_conn:
            if replace_target:
                for table in reversed(tables):
                    target_conn.execute(table.delete())
            else:
                non_empty_tables = [
                    table.name
                    for table in tables
                    if _count_rows(target_conn, table) > 0
                ]
                if non_empty_tables:
                    joined = ", ".join(non_empty_tables)
                    raise ValueError(f"目标数据库已存在数据，请先清空或使用 replace_target=True: {joined}")

            copied_tables: dict[str, int] = {}
            total_rows = 0

            for table in tables:
                result = source_conn.execution_options(stream_results=True).execute(select(table))
                copied_rows = 0
                while True:
                    rows = result.mappings().fetchmany(batch_size)
                    if not rows:
                        break
                    payload = [dict(row) for row in rows]
                    target_conn.execute(table.insert(), payload)
                    copied_rows += len(payload)
                copied_tables[table.name] = copied_rows
                total_rows += copied_rows

            _sync_postgres_sequences(target_conn, tables)

        return {
            "source_url": source_url,
            "target_url": target_url,
            "tables": copied_tables,
            "total_rows": total_rows,
        }
    finally:
        source_engine.dispose()
        target_engine.dispose()


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="将 SQLite 数据迁移到 PostgreSQL")
    parser.add_argument("--sqlite", required=True, help="SQLite 文件路径或 sqlite:/// URL")
    parser.add_argument("--target", required=True, help="目标数据库 URL，通常为 PostgreSQL")
    parser.add_argument("--replace-target", action="store_true", help="覆盖目标库现有数据")
    parser.add_argument("--batch-size", type=int, default=500, help="批量写入大小")
    return parser


def main() -> int:
    parser = _build_parser()
    args = parser.parse_args()

    summary = migrate_sqlite_to_database(
        args.sqlite,
        args.target,
        replace_target=args.replace_target,
        batch_size=args.batch_size,
    )

    print("迁移完成")
    print(f"源数据库: {summary['source_url']}")
    print(f"目标数据库: {summary['target_url']}")
    for table_name, row_count in summary["tables"].items():
        print(f"- {table_name}: {row_count}")
    print(f"总计行数: {summary['total_rows']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
