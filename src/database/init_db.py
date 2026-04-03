"""
数据库初始化和初始化数据
"""

from .session import init_database
from .models import Base, Account
from .session import DatabaseSessionManager
from ..config.constants import AccountStatus


def initialize_database(database_url: str = None):
    """
    初始化数据库
    创建所有表并设置默认配置
    """
    # 初始化数据库连接和表
    db_manager = init_database(database_url)

    # 创建表
    db_manager.create_tables()

    # 初始化默认设置（从 settings 模块导入以避免循环导入）
    from ..config.settings import init_default_settings
    init_default_settings()

    return db_manager


def reset_database(database_url: str = None):
    """
    重置数据库（删除所有表并重新创建）
    警告：会丢失所有数据！
    """
    db_manager = init_database(database_url)

    # 删除所有表
    db_manager.drop_tables()
    print("已删除所有表")

    # 重新创建所有表
    db_manager.create_tables()
    print("已重新创建所有表")

    # 初始化默认设置
    from ..config.settings import init_default_settings
    init_default_settings()

    print("数据库重置完成")
    return db_manager


def check_database_connection(database_url: str = None) -> bool:
    """
    检查数据库连接是否正常
    """
    try:
        db_manager = init_database(database_url)
        with db_manager.get_db() as db:
            # 尝试执行一个简单的查询
            db.execute("SELECT 1")
        print("数据库连接正常")
        return True
    except Exception as e:
        print(f"数据库连接失败: {e}")
        return False


def _derive_partial_account_status(account: Account):
    refresh_token = str(getattr(account, "refresh_token", "") or "").strip()
    if refresh_token:
        return None, None

    password = str(getattr(account, "password", "") or "").strip()
    source = str(getattr(account, "source", "") or "").strip().lower()
    extra_data = dict(getattr(account, "extra_data", {}) or {})

    if source == "login" and not password:
        return AccountStatus.LOGIN_INCOMPLETE.value, "repair_missing_login_password"
    if extra_data.get("existing_account_detected") and not password:
        return AccountStatus.LOGIN_INCOMPLETE.value, "repair_missing_login_password"
    return AccountStatus.TOKEN_PENDING.value, "repair_missing_refresh_token"


def repair_partial_account_statuses(database_url: str = None, dry_run: bool = False):
    """
    将历史上误落为 active 的半成品账号纠偏为 token_pending/login_incomplete。
    """
    manager = DatabaseSessionManager(database_url)
    summary = {
        "checked": 0,
        "updated": 0,
        "token_pending": 0,
        "login_incomplete": 0,
        "dry_run": bool(dry_run),
    }

    with manager.session_scope() as session:
        accounts = session.query(Account).filter(
            Account.status == AccountStatus.ACTIVE.value
        ).all()

        for account in accounts:
            summary["checked"] += 1
            next_status, reason = _derive_partial_account_status(account)
            if not next_status:
                continue

            summary["updated"] += 1
            summary[next_status] += 1
            if dry_run:
                continue

            extra_data = dict(account.extra_data or {})
            extra_data["account_status_reason"] = reason
            extra_data["token_pending"] = next_status == AccountStatus.TOKEN_PENDING.value
            extra_data["login_incomplete"] = next_status == AccountStatus.LOGIN_INCOMPLETE.value
            account.status = next_status
            account.extra_data = extra_data

    return summary


if __name__ == "__main__":
    # 当直接运行此脚本时，初始化数据库
    import argparse

    parser = argparse.ArgumentParser(description="数据库初始化脚本")
    parser.add_argument("--reset", action="store_true", help="重置数据库（删除所有数据）")
    parser.add_argument("--check", action="store_true", help="检查数据库连接")
    parser.add_argument("--repair-partial", action="store_true", help="修复误落为 active 的半成品账号状态")
    parser.add_argument("--dry-run", action="store_true", help="仅预览修复结果，不实际写入")
    parser.add_argument("--url", help="数据库连接字符串")

    args = parser.parse_args()

    if args.check:
        check_database_connection(args.url)
    elif args.repair_partial:
        summary = repair_partial_account_statuses(args.url, dry_run=args.dry_run)
        print(
            "修复完成:"
            f" checked={summary['checked']}"
            f" updated={summary['updated']}"
            f" token_pending={summary['token_pending']}"
            f" login_incomplete={summary['login_incomplete']}"
            f" dry_run={summary['dry_run']}"
        )
    elif args.reset:
        confirm = input("警告：这将删除所有数据！确认重置？(y/N): ")
        if confirm.lower() == 'y':
            reset_database(args.url)
        else:
            print("操作已取消")
    else:
        initialize_database(args.url)
