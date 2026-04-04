"""
数据库会话管理
"""

from contextlib import contextmanager
from typing import Generator
from sqlalchemy import create_engine, text
from sqlalchemy import event
from sqlalchemy.orm import sessionmaker, Session
from sqlalchemy.exc import SQLAlchemyError
import os
import logging

from .models import Base
from . import team_models  # noqa: F401  # 确保 Team 模型在建表前注册到同一 metadata

logger = logging.getLogger(__name__)


def _build_sqlalchemy_url(database_url: str) -> str:
    if database_url.startswith("postgresql://"):
        return "postgresql+psycopg://" + database_url[len("postgresql://"):]
    if database_url.startswith("postgres://"):
        return "postgresql+psycopg://" + database_url[len("postgres://"):]
    return database_url


class DatabaseSessionManager:
    """数据库会话管理器"""

    def __init__(self, database_url: str = None):
        if database_url is None:
            env_url = os.environ.get("APP_DATABASE_URL") or os.environ.get("DATABASE_URL")
            if env_url:
                database_url = env_url
            else:
                # 优先使用 APP_DATA_DIR 环境变量（PyInstaller 打包后由 webui.py 设置）
                data_dir = os.environ.get('APP_DATA_DIR') or os.path.join(
                    os.path.dirname(os.path.dirname(os.path.dirname(__file__))),
                    'data'
                )
                db_path = os.path.join(data_dir, 'database.db')
                # 确保目录存在
                os.makedirs(data_dir, exist_ok=True)
                database_url = f"sqlite:///{db_path}"

        self.database_url = _build_sqlalchemy_url(database_url)
        connect_args = {}
        if self.database_url.startswith("sqlite"):
            connect_args = {
                "check_same_thread": False,
                "timeout": 30,
            }
        self.engine = create_engine(
            self.database_url,
            connect_args=connect_args,
            echo=False,  # 设置为 True 可以查看所有 SQL 语句
            pool_pre_ping=True  # 连接池预检查
        )
        if self.database_url.startswith("sqlite"):
            self._configure_sqlite_pragmas()
        self.SessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=self.engine)

    def _configure_sqlite_pragmas(self) -> None:
        """为 SQLite 打开更适合并发场景的 pragma。"""

        @event.listens_for(self.engine, "connect")
        def _set_sqlite_pragmas(dbapi_connection, _connection_record):
            cursor = dbapi_connection.cursor()
            try:
                cursor.execute("PRAGMA journal_mode=WAL")
                cursor.execute("PRAGMA synchronous=NORMAL")
                cursor.execute("PRAGMA busy_timeout=30000")
                cursor.execute("PRAGMA foreign_keys=ON")
            finally:
                cursor.close()

    def get_db(self) -> Generator[Session, None, None]:
        """
        获取数据库会话的上下文管理器
        使用示例:
            with get_db() as db:
                # 使用 db 进行数据库操作
                pass
        """
        db = self.SessionLocal()
        try:
            yield db
        finally:
            db.close()

    @contextmanager
    def session_scope(self) -> Generator[Session, None, None]:
        """
        事务作用域上下文管理器
        使用示例:
            with session_scope() as session:
                # 数据库操作
                pass
        """
        session = self.SessionLocal()
        try:
            yield session
            session.commit()
        except Exception as e:
            session.rollback()
            raise e
        finally:
            session.close()

    def create_tables(self):
        """创建所有表"""
        Base.metadata.create_all(bind=self.engine)

    def drop_tables(self):
        """删除所有表（谨慎使用）"""
        Base.metadata.drop_all(bind=self.engine)

    def migrate_tables(self):
        """
        数据库迁移 - 添加缺失的列
        用于在不删除数据的情况下更新表结构
        """
        if not self.database_url.startswith("sqlite"):
            logger.info("非 SQLite 数据库，跳过自动迁移")
            return

        # 需要检查和添加的新列
        migrations = [
            # (表名, 列名, 列类型)
            ("accounts", "cpa_uploaded", "BOOLEAN DEFAULT 0"),
            ("accounts", "cpa_uploaded_at", "DATETIME"),
            ("accounts", "sub2api_uploaded", "BOOLEAN DEFAULT 0"),
            ("accounts", "sub2api_uploaded_at", "DATETIME"),
            ("accounts", "source", "VARCHAR(20) DEFAULT 'register'"),
            ("accounts", "subscription_type", "VARCHAR(20)"),
            ("accounts", "subscription_at", "DATETIME"),
            ("accounts", "cookies", "TEXT"),
            ("cpa_services", "proxy_url", "VARCHAR(1000)"),
            ("sub2api_services", "target_type", "VARCHAR(50) DEFAULT 'sub2api'"),
            ("proxies", "is_default", "BOOLEAN DEFAULT 0"),
            ("bind_card_tasks", "checkout_session_id", "VARCHAR(120)"),
            ("bind_card_tasks", "publishable_key", "VARCHAR(255)"),
            ("bind_card_tasks", "client_secret", "TEXT"),
            ("bind_card_tasks", "bind_mode", "VARCHAR(30) DEFAULT 'semi_auto'"),
        ]

        # 确保新表存在（create_tables 已处理，此处兜底）
        Base.metadata.create_all(bind=self.engine)

        with self.engine.connect() as conn:
            # 数据迁移：将旧的 custom_domain 记录统一为 moe_mail
            try:
                conn.execute(text("UPDATE email_services SET service_type='moe_mail' WHERE service_type='custom_domain'"))
                conn.execute(text("UPDATE accounts SET email_service='moe_mail' WHERE email_service='custom_domain'"))
                conn.commit()
            except Exception as e:
                logger.warning(f"迁移 custom_domain -> moe_mail 时出错: {e}")

            for table_name, column_name, column_type in migrations:
                try:
                    # 检查列是否存在
                    result = conn.execute(text(
                        f"SELECT * FROM pragma_table_info('{table_name}') WHERE name='{column_name}'"
                    ))
                    if result.fetchone() is None:
                        # 列不存在，添加它
                        logger.info(f"添加列 {table_name}.{column_name}")
                        conn.execute(text(
                            f"ALTER TABLE {table_name} ADD COLUMN {column_name} {column_type}"
                        ))
                        conn.commit()
                        logger.info(f"成功添加列 {table_name}.{column_name}")
                except Exception as e:
                    logger.warning(f"迁移列 {table_name}.{column_name} 时出错: {e}")

            self._migrate_team_tasks_scope_columns(conn)

    def _sqlite_column_exists(self, conn, table_name: str, column_name: str) -> bool:
        result = conn.execute(
            text(f"SELECT * FROM pragma_table_info('{table_name}') WHERE name='{column_name}'")
        )
        return result.fetchone() is not None

    def _sqlite_index_exists(self, conn, table_name: str, index_name: str) -> bool:
        result = conn.execute(text(f"PRAGMA index_list('{table_name}')"))
        return any(row[1] == index_name for row in result.fetchall())

    def _migrate_team_tasks_scope_columns(self, conn) -> None:
        """为旧版 team_tasks 表补充 scope 字段并建立唯一索引。"""
        team_task_columns = [
            ("scope_type", "VARCHAR(20) NOT NULL DEFAULT ''"),
            ("scope_id", "VARCHAR(100) NOT NULL DEFAULT ''"),
            ("active_scope_key", "VARCHAR(150)"),
        ]
        for column_name, column_type in team_task_columns:
            try:
                if not self._sqlite_column_exists(conn, "team_tasks", column_name):
                    logger.info(f"添加列 team_tasks.{column_name}")
                    conn.execute(
                        text(f"ALTER TABLE team_tasks ADD COLUMN {column_name} {column_type}")
                    )
                    conn.commit()
            except Exception as e:
                logger.warning(f"迁移列 team_tasks.{column_name} 时出错: {e}")

        try:
            conn.execute(
                text(
                    """
                    UPDATE team_tasks
                    SET scope_type = CASE
                        WHEN team_id IS NOT NULL THEN 'team'
                        WHEN owner_account_id IS NOT NULL THEN 'owner'
                        ELSE ''
                    END
                    WHERE scope_type IS NULL OR TRIM(scope_type) = ''
                    """
                )
            )
            conn.execute(
                text(
                    """
                    UPDATE team_tasks
                    SET scope_id = CASE
                        WHEN team_id IS NOT NULL THEN CAST(team_id AS TEXT)
                        WHEN owner_account_id IS NOT NULL THEN CAST(owner_account_id AS TEXT)
                        ELSE ''
                    END
                    WHERE scope_id IS NULL OR TRIM(scope_id) = ''
                    """
                )
            )
            conn.execute(
                text(
                    """
                    UPDATE team_tasks
                    SET active_scope_key = CASE
                        WHEN status IN ('completed', 'failed', 'cancelled') THEN NULL
                        WHEN team_id IS NOT NULL THEN 'team:' || CAST(team_id AS TEXT)
                        WHEN owner_account_id IS NOT NULL THEN 'owner:' || CAST(owner_account_id AS TEXT)
                        ELSE NULL
                    END
                    WHERE active_scope_key IS NULL OR TRIM(active_scope_key) = ''
                    """
                )
            )
            conn.commit()
        except Exception as e:
            logger.warning(f"回填 team_tasks scope 字段时出错: {e}")

        try:
            index_name = "ix_team_tasks_active_scope_key"
            if not self._sqlite_index_exists(conn, "team_tasks", index_name):
                conn.execute(
                    text(
                        f"CREATE UNIQUE INDEX {index_name} ON team_tasks (active_scope_key)"
                    )
                )
                conn.commit()
        except Exception as e:
            logger.warning(f"创建 team_tasks.active_scope_key 唯一索引时出错: {e}")


# 全局数据库会话管理器实例
_db_manager: DatabaseSessionManager = None


def init_database(database_url: str = None) -> DatabaseSessionManager:
    """
    初始化数据库会话管理器
    """
    global _db_manager
    if _db_manager is None:
        _db_manager = DatabaseSessionManager(database_url)
        _db_manager.create_tables()
        # 执行数据库迁移
        _db_manager.migrate_tables()
    return _db_manager


def get_session_manager() -> DatabaseSessionManager:
    """
    获取数据库会话管理器
    """
    if _db_manager is None:
        raise RuntimeError("数据库未初始化，请先调用 init_database()")
    return _db_manager


@contextmanager
def get_db() -> Generator[Session, None, None]:
    """
    获取数据库会话的快捷函数
    """
    manager = get_session_manager()
    db = manager.SessionLocal()
    try:
        yield db
    finally:
        db.close()
