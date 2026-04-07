"""
注册任务 API 路由
"""

import asyncio
import logging
import uuid
import random
import threading
from datetime import datetime
from typing import List, Optional, Dict, Tuple
from sqlalchemy.exc import OperationalError

from fastapi import APIRouter, HTTPException, Query, BackgroundTasks
from pydantic import BaseModel, Field

from ...database import crud
from ...database.session import get_db
from ...database.models import RegistrationTask, Proxy
from ...core.register import RegistrationCancelledError, RegistrationEngine, RegistrationResult
from ...services import EmailServiceFactory, EmailServiceType
from ...config.settings import get_settings
from ..task_manager import task_manager

logger = logging.getLogger(__name__)
router = APIRouter()

# 任务存储（简单的内存存储，生产环境应使用 Redis）
running_tasks: dict = {}
# 批量任务存储
batch_tasks: Dict[str, dict] = {}
_outlook_service_claim_lock = threading.Lock()
_BATCH_LOG_HISTORY_LIMIT = 1000


# ============== Proxy Helper Functions ==============

def get_proxy_for_registration(db) -> Tuple[Optional[str], Optional[int]]:
    """
    获取用于注册的代理

    策略：
    1. 优先从代理列表中随机选择一个启用的代理
    2. 如果代理列表为空且启用了动态代理，调用动态代理 API 获取
    3. 否则使用系统设置中的静态默认代理

    Returns:
        Tuple[proxy_url, proxy_id]: 代理 URL 和代理 ID（如果来自代理列表）
    """
    # 先尝试从代理列表中获取
    proxy = crud.get_random_proxy(db)
    if proxy:
        return proxy.proxy_url, proxy.id

    # 代理列表为空，尝试动态代理或静态代理
    from ...core.dynamic_proxy import get_proxy_url_for_task
    proxy_url = get_proxy_url_for_task()
    if proxy_url:
        return proxy_url, None

    return None, None


def update_proxy_usage(db, proxy_id: Optional[int]):
    """更新代理的使用时间"""
    if proxy_id:
        crud.update_proxy_last_used(db, proxy_id)


def _reserve_available_outlook_service(db, task_uuid: str):
    """为任务预留一个尚未注册且未被其他执行中任务占用的 Outlook 账户。"""
    from ...database.models import EmailService as EmailServiceModel, Account

    with _outlook_service_claim_lock:
        task = crud.get_registration_task_by_uuid(db, task_uuid)
        if task is None:
            raise ValueError(f"任务不存在: {task_uuid}")

        if task.email_service_id:
            db_service = db.query(EmailServiceModel).filter(
                EmailServiceModel.id == task.email_service_id,
                EmailServiceModel.enabled == True,
            ).first()
            if db_service:
                return db_service

        claimed_rows = db.query(RegistrationTask.email_service_id).filter(
            RegistrationTask.email_service_id.isnot(None),
            RegistrationTask.task_uuid != task_uuid,
            RegistrationTask.status.in_(("pending", "running", "paused", "cancelling")),
        ).all()
        claimed_service_ids = {int(row[0]) for row in claimed_rows if row and row[0] is not None}

        outlook_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "outlook",
            EmailServiceModel.enabled == True,
        ).order_by(EmailServiceModel.priority.asc()).all()

        if not outlook_services:
            raise ValueError("没有可用的 Outlook 账户，请先在设置中导入账户")

        for svc in outlook_services:
            email = svc.config.get("email") if svc.config else None
            if not email:
                continue

            if svc.id in claimed_service_ids:
                logger.info(f"跳过已被其他任务预留的 Outlook 账户: {email} (ID: {svc.id})")
                continue

            existing = db.query(Account).filter(Account.email == email).first()
            if existing:
                logger.info(f"跳过已注册的 Outlook 账户: {email}")
                continue

            task.email_service_id = svc.id
            db.commit()
            db.refresh(task)
            logger.info(f"预留未注册的 Outlook 账户: {email} (任务: {task_uuid})")
            return svc

        raise ValueError("所有 Outlook 账户都已注册过 OpenAI 账号或已被其他任务占用，请稍后重试")


# ============== Pydantic Models ==============

class RegistrationTaskCreate(BaseModel):
    """创建注册任务请求"""
    email_service_type: str = "tempmail"
    chatgpt_registration_mode: str = "refresh_token"
    proxy: Optional[str] = None
    email_service_config: Optional[dict] = None
    email_service_id: Optional[int] = None
    auto_upload_cpa: bool = False
    cpa_service_ids: List[int] = []  # 指定 CPA 服务 ID 列表，空则取第一个启用的
    auto_upload_sub2api: bool = False
    sub2api_service_ids: List[int] = []  # 指定 Sub2API 服务 ID 列表
    auto_upload_tm: bool = False
    tm_service_ids: List[int] = []  # 指定 TM 服务 ID 列表


class BatchRegistrationRequest(BaseModel):
    """批量注册请求"""
    count: int = 1
    email_service_type: str = "tempmail"
    chatgpt_registration_mode: str = "refresh_token"
    proxy: Optional[str] = None
    email_service_config: Optional[dict] = None
    email_service_id: Optional[int] = None
    interval_min: int = 5
    interval_max: int = 30
    concurrency: int = 1
    token_completion_concurrency: Optional[int] = None
    mode: str = "pipeline"
    auto_upload_cpa: bool = False
    cpa_service_ids: List[int] = []
    auto_upload_sub2api: bool = False
    sub2api_service_ids: List[int] = []
    auto_upload_tm: bool = False
    tm_service_ids: List[int] = []


class RegistrationTaskResponse(BaseModel):
    """注册任务响应"""
    id: int
    task_uuid: str
    status: str
    email_service_id: Optional[int] = None
    proxy: Optional[str] = None
    logs: Optional[str] = None
    result: Optional[dict] = None
    error_message: Optional[str] = None
    created_at: Optional[str] = None
    started_at: Optional[str] = None
    completed_at: Optional[str] = None

    class Config:
        from_attributes = True


class BatchRegistrationResponse(BaseModel):
    """批量注册响应"""
    batch_id: str
    count: int
    tasks: List[RegistrationTaskResponse]


class TaskListResponse(BaseModel):
    """任务列表响应"""
    total: int
    tasks: List[RegistrationTaskResponse]


# ============== Outlook 批量注册模型 ==============

class OutlookAccountForRegistration(BaseModel):
    """可用于注册的 Outlook 账户"""
    id: int                      # EmailService 表的 ID
    email: str
    name: str
    has_oauth: bool              # 是否有 OAuth 配置
    is_registered: bool          # 是否已注册
    has_refresh_token: bool = False
    needs_token_refresh: bool = False
    is_registration_complete: bool = False
    registered_account_id: Optional[int] = None


class OutlookAccountsListResponse(BaseModel):
    """Outlook 账户列表响应"""
    total: int
    registered_count: int        # 已注册数量
    unregistered_count: int      # 未注册数量
    accounts: List[OutlookAccountForRegistration]


class OutlookBatchRegistrationRequest(BaseModel):
    """Outlook 批量注册请求"""
    service_ids: List[int]
    proxy: Optional[str] = None
    interval_min: int = 5
    interval_max: int = 30
    concurrency: int = 1
    token_completion_concurrency: Optional[int] = None
    mode: str = "pipeline"
    auto_upload_cpa: bool = False
    cpa_service_ids: List[int] = []
    auto_upload_sub2api: bool = False
    sub2api_service_ids: List[int] = []
    auto_upload_tm: bool = False
    tm_service_ids: List[int] = []


class OutlookBatchRegistrationResponse(BaseModel):
    """Outlook 批量注册响应"""
    batch_id: str
    total: int                   # 总数
    skipped: int                 # 跳过数（已注册）
    to_register: int             # 待注册数
    service_ids: List[int]       # 实际要注册的服务 ID


# ============== Helper Functions ==============

def _has_refresh_token(account: Optional[object]) -> bool:
    if not account:
        return False
    return bool(str(getattr(account, "refresh_token", "") or "").strip())


def _is_account_registration_complete(account: Optional[object]) -> bool:
    if not account:
        return False
    return _has_refresh_token(account)


def _needs_token_refresh(account: Optional[object]) -> bool:
    return bool(account) and not _is_account_registration_complete(account)


def _derive_outlook_execution_state(account: Optional[object]) -> str:
    # 当前注册执行状态只看账号是否存在、以及 refresh token 是否完整；
    # 不引入 account.status 的额外业务语义。
    if _is_account_registration_complete(account):
        return "registered_complete"
    if account:
        return "registered_needs_token_refresh"
    return "unregistered"


def task_to_response(task: RegistrationTask) -> RegistrationTaskResponse:
    """转换任务模型为响应"""
    return RegistrationTaskResponse(
        id=task.id,
        task_uuid=task.task_uuid,
        status=task.status,
        email_service_id=task.email_service_id,
        proxy=task.proxy,
        logs=task.logs,
        result=task.result,
        error_message=task.error_message,
        created_at=task.created_at.isoformat() if task.created_at else None,
        started_at=task.started_at.isoformat() if task.started_at else None,
        completed_at=task.completed_at.isoformat() if task.completed_at else None,
    )


def _normalize_email_service_config(
    service_type: EmailServiceType,
    config: Optional[dict],
    proxy_url: Optional[str] = None
) -> dict:
    """按服务类型兼容旧字段名，避免不同服务的配置键互相污染。"""
    normalized = config.copy() if config else {}

    if 'api_url' in normalized and 'base_url' not in normalized:
        normalized['base_url'] = normalized.pop('api_url')

    if service_type == EmailServiceType.MOE_MAIL:
        if 'domain' in normalized and 'default_domain' not in normalized:
            normalized['default_domain'] = normalized.pop('domain')
    elif service_type == EmailServiceType.YYDS_MAIL:
        if 'domain' in normalized and 'default_domain' not in normalized:
            normalized['default_domain'] = normalized.pop('domain')
    elif service_type in (EmailServiceType.TEMP_MAIL, EmailServiceType.FREEMAIL):
        if 'default_domain' in normalized and 'domain' not in normalized:
            normalized['domain'] = normalized.pop('default_domain')
    elif service_type == EmailServiceType.DUCK_MAIL:
        if 'domain' in normalized and 'default_domain' not in normalized:
            normalized['default_domain'] = normalized.pop('domain')
    elif service_type == EmailServiceType.LUCKMAIL:
        if 'domain' in normalized and 'preferred_domain' not in normalized:
            normalized['preferred_domain'] = normalized.pop('domain')

    if proxy_url and 'proxy_url' not in normalized:
        normalized['proxy_url'] = proxy_url

    return normalized


def _snapshot_runtime_logs(task_uuid: str) -> Optional[str]:
    """优先使用内存日志快照，减少高并发下的数据库写放大。"""
    runtime_logs = task_manager.get_logs(task_uuid)
    if runtime_logs:
        return "\n".join(runtime_logs)
    return None


def _resolve_log_window(
    logs: List[str],
    *,
    offset: int = 0,
    base_index: int = 0,
) -> tuple[List[str], int, int]:
    """按全局偏移切出增量日志窗口。"""
    try:
        safe_base_index = max(0, int(base_index or 0))
    except (TypeError, ValueError):
        safe_base_index = 0
    try:
        safe_offset = max(0, int(offset or 0))
    except (TypeError, ValueError):
        safe_offset = 0
    safe_offset = max(safe_offset, safe_base_index)
    window_end = safe_base_index + len(logs)
    if safe_offset > window_end:
        safe_offset = window_end
    slice_start = max(0, safe_offset - safe_base_index)
    return logs[slice_start:], safe_offset, window_end


def _default_resume_status(task: Optional[RegistrationTask]) -> str:
    """推断暂停后的默认恢复状态。"""
    if task is None:
        return "running"
    if getattr(task, "started_at", None):
        return "running"
    return "pending"


def _resolve_resume_status(task_uuid: str, task: Optional[RegistrationTask]) -> str:
    """优先读取运行时记录的恢复目标状态，否则退回数据库推断。"""
    return task_manager.get_resume_status(task_uuid, default=_default_resume_status(task))


def _pause_single_task_record(db, task: RegistrationTask) -> str:
    """暂停单任务并返回写入后的状态。"""
    resume_status = task.status if task.status in {"pending", "running"} else _default_resume_status(task)
    task_manager.pause_task(task.task_uuid, resume_status=resume_status)
    crud.update_registration_task(db, task.task_uuid, status="paused")
    task_manager.update_status(task.task_uuid, "paused")
    return "paused"


def _resume_single_task_record(db, task: RegistrationTask) -> str:
    """恢复单任务并返回写入后的状态。"""
    resume_status = _resolve_resume_status(task.task_uuid, task)
    task_manager.resume_task(task.task_uuid, resume_status=resume_status)
    crud.update_registration_task(db, task.task_uuid, status=resume_status)
    task_manager.update_status(task.task_uuid, resume_status)
    return resume_status


def _update_batch_child_task_statuses(db, batch_id: str, *, action: str):
    """批量同步子任务的暂停/恢复状态。"""
    for task_uuid in batch_tasks.get(batch_id, {}).get("task_uuids", []):
        task = crud.get_registration_task(db, task_uuid)
        if task is None:
            continue
        if action == "pause":
            if task.status in {"pending", "running"}:
                _pause_single_task_record(db, task)
        elif action == "resume":
            if task.status == "paused":
                _resume_single_task_record(db, task)


async def _wait_if_batch_paused(batch_id: str):
    """批量调度器在暂停期间挂起，恢复或取消后继续。"""
    while batch_tasks.get(batch_id, {}).get("paused") and not task_manager.is_batch_cancelled(batch_id):
        await asyncio.sleep(0.2)


async def _sleep_with_batch_controls(batch_id: str, seconds: float):
    """带暂停/取消感知的异步等待。"""
    remaining = max(0.0, float(seconds or 0.0))
    while remaining > 0:
        if task_manager.is_batch_cancelled(batch_id):
            return
        await _wait_if_batch_paused(batch_id)
        step = min(0.2, remaining)
        await asyncio.sleep(step)
        remaining -= step


def _finalize_task_record(
    db,
    task_uuid: str,
    *,
    status: str,
    completed_at: Optional[datetime] = None,
    result: Optional[dict] = None,
    error_message: Optional[str] = None,
):
    """统一写入任务终态，并带上运行期日志快照。"""
    payload: dict[str, object] = {
        "status": status,
        "completed_at": completed_at or datetime.utcnow(),
    }
    runtime_logs = _snapshot_runtime_logs(task_uuid)
    if runtime_logs is not None:
        payload["logs"] = runtime_logs
    if result is not None:
        payload["result"] = result
    if error_message is not None:
        payload["error_message"] = error_message
    return crud.update_registration_task(db, task_uuid, **payload)


def _safe_update_registration_task(
    db,
    task_uuid: str,
    *,
    context: str,
    **kwargs,
):
    """对非关键元数据更新做锁冲突降级，避免一次写锁把整个注册线程打死。"""
    try:
        return crud.update_registration_task(db, task_uuid, **kwargs)
    except OperationalError as exc:
        message = str(exc or "").lower()
        if "database is locked" not in message and "database table is locked" not in message:
            raise
        logger.warning("任务 %s 在 %s 时遇到 SQLite 写锁，已跳过本次元数据更新: %s", task_uuid, context, exc)
        return None


def _run_sync_registration_task(task_uuid: str, email_service_type: str, proxy: Optional[str], email_service_config: Optional[dict], email_service_id: Optional[int] = None, log_prefix: str = "", batch_id: str = "", auto_upload_cpa: bool = False, cpa_service_ids: List[int] = None, auto_upload_sub2api: bool = False, sub2api_service_ids: List[int] = None, auto_upload_tm: bool = False, tm_service_ids: List[int] = None, token_completion_concurrency: int = 1, chatgpt_registration_mode: str = "refresh_token"):
    """
    在线程池中执行的同步注册任务

    这个函数会被 run_in_executor 调用，运行在独立线程中
    """
    with get_db() as db:
        try:
            task_manager.wait_if_paused(task_uuid)
            # 检查是否已取消
            if task_manager.is_cancelled(task_uuid):
                logger.info(f"任务 {task_uuid} 已取消，跳过执行")
                _finalize_task_record(db, task_uuid, status="cancelled")
                task_manager.update_status(task_uuid, "cancelled")
                return

            task = crud.get_registration_task(db, task_uuid)
            if not task:
                logger.error(f"任务不存在: {task_uuid}")
                return

            # 优先使用任务已绑定的邮箱服务，避免批量 Outlook 任务在并发下抢同一邮箱。
            bound_email_service_id = email_service_id or getattr(task, "email_service_id", None)

            # 确定使用的代理
            # 如果前端传入了代理参数，使用传入的
            # 否则从代理列表或系统设置中获取
            actual_proxy_url = proxy
            proxy_id = None

            if not actual_proxy_url:
                actual_proxy_url, proxy_id = get_proxy_for_registration(db)
                if actual_proxy_url:
                    logger.info(f"任务 {task_uuid} 使用代理: {actual_proxy_url[:50]}...")

            # 合并早期元数据写入，减少并发下的 SQLite commit 次数
            updated_task = _safe_update_registration_task(
                db,
                task_uuid,
                context="切换 running 状态并写入 proxy",
                status="running",
                started_at=datetime.utcnow(),
                proxy=actual_proxy_url,
            )
            if updated_task is not None:
                task = updated_task

            # 更新 TaskManager 状态
            task_manager.update_status(task_uuid, "running")

            # 创建邮箱服务
            service_type = EmailServiceType(email_service_type)
            settings = get_settings()
            requested_token_completion_concurrency = max(1, int(token_completion_concurrency or 1))
            token_completion_max_concurrency = max(
                0,
                int(getattr(settings, "registration_token_completion_max_concurrency", 0) or 0),
            )

            # 优先使用数据库中配置的邮箱服务
            if bound_email_service_id:
                from ...database.models import EmailService as EmailServiceModel
                db_service = db.query(EmailServiceModel).filter(
                    EmailServiceModel.id == bound_email_service_id,
                    EmailServiceModel.enabled == True
                ).first()

                if db_service:
                    service_type = EmailServiceType(db_service.service_type)
                    config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                    # 更新任务关联的邮箱服务
                    _safe_update_registration_task(db, task_uuid, context="绑定邮箱服务", email_service_id=db_service.id)
                    logger.info(f"使用数据库邮箱服务: {db_service.name} (ID: {db_service.id}, 类型: {service_type.value})")
                else:
                    raise ValueError(f"邮箱服务不存在或已禁用: {bound_email_service_id}")
            else:
                # 使用默认配置或传入的配置
                if service_type == EmailServiceType.TEMPMAIL:
                    if not settings.tempmail_enabled:
                        raise ValueError("Tempmail.lol 渠道已禁用，请先在邮箱服务页面启用")
                    config = {
                        "base_url": settings.tempmail_base_url,
                        "timeout": settings.tempmail_timeout,
                        "max_retries": settings.tempmail_max_retries,
                        "proxy_url": actual_proxy_url,
                    }
                elif service_type == EmailServiceType.YYDS_MAIL:
                    api_key = settings.yyds_mail_api_key.get_secret_value() if settings.yyds_mail_api_key else ""
                    if not settings.yyds_mail_enabled or not api_key:
                        raise ValueError("YYDS Mail 渠道未启用或未配置 API Key，请先在邮箱服务页面配置")
                    config = {
                        "base_url": settings.yyds_mail_base_url,
                        "api_key": api_key,
                        "default_domain": settings.yyds_mail_default_domain,
                        "timeout": settings.yyds_mail_timeout,
                        "max_retries": settings.yyds_mail_max_retries,
                        "proxy_url": actual_proxy_url,
                    }
                elif service_type == EmailServiceType.MOE_MAIL:
                    # 检查数据库中是否有可用的自定义域名服务
                    from ...database.models import EmailService as EmailServiceModel
                    db_service = db.query(EmailServiceModel).filter(
                        EmailServiceModel.service_type == "moe_mail",
                        EmailServiceModel.enabled == True
                    ).order_by(EmailServiceModel.priority.asc()).first()

                    if db_service and db_service.config:
                        config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                        _safe_update_registration_task(db, task_uuid, context="绑定 moe_mail 服务", email_service_id=db_service.id)
                        logger.info(f"使用数据库自定义域名服务: {db_service.name}")
                    elif settings.custom_domain_base_url and settings.custom_domain_api_key:
                        config = {
                            "base_url": settings.custom_domain_base_url,
                            "api_key": settings.custom_domain_api_key.get_secret_value() if settings.custom_domain_api_key else "",
                            "proxy_url": actual_proxy_url,
                        }
                    else:
                        raise ValueError("没有可用的自定义域名邮箱服务，请先在设置中配置")
                elif service_type == EmailServiceType.OUTLOOK:
                    selected_service = _reserve_available_outlook_service(db, task_uuid)
                    if selected_service and selected_service.config:
                        config = selected_service.config.copy()
                        logger.info(f"使用数据库 Outlook 账户: {selected_service.name}")
                    else:
                        raise ValueError("没有可用的 Outlook 账户，请先在设置中导入账户")
                elif service_type == EmailServiceType.DUCK_MAIL:
                    from ...database.models import EmailService as EmailServiceModel

                    db_service = db.query(EmailServiceModel).filter(
                        EmailServiceModel.service_type == "duck_mail",
                        EmailServiceModel.enabled == True
                    ).order_by(EmailServiceModel.priority.asc()).first()

                    if db_service and db_service.config:
                        config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                        _safe_update_registration_task(db, task_uuid, context="绑定 duck_mail 服务", email_service_id=db_service.id)
                        logger.info(f"使用数据库 DuckMail 服务: {db_service.name}")
                    else:
                        raise ValueError("没有可用的 DuckMail 邮箱服务，请先在邮箱服务页面添加服务")
                elif service_type == EmailServiceType.FREEMAIL:
                    from ...database.models import EmailService as EmailServiceModel

                    db_service = db.query(EmailServiceModel).filter(
                        EmailServiceModel.service_type == "freemail",
                        EmailServiceModel.enabled == True
                    ).order_by(EmailServiceModel.priority.asc()).first()

                    if db_service and db_service.config:
                        config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                        _safe_update_registration_task(db, task_uuid, context="绑定 freemail 服务", email_service_id=db_service.id)
                        logger.info(f"使用数据库 Freemail 服务: {db_service.name}")
                    else:
                        raise ValueError("没有可用的 Freemail 邮箱服务，请先在邮箱服务页面添加服务")
                elif service_type == EmailServiceType.IMAP_MAIL:
                    from ...database.models import EmailService as EmailServiceModel

                    db_service = db.query(EmailServiceModel).filter(
                        EmailServiceModel.service_type == "imap_mail",
                        EmailServiceModel.enabled == True
                    ).order_by(EmailServiceModel.priority.asc()).first()

                    if db_service and db_service.config:
                        config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                        _safe_update_registration_task(db, task_uuid, context="绑定 imap_mail 服务", email_service_id=db_service.id)
                        logger.info(f"使用数据库 IMAP 邮箱服务: {db_service.name}")
                    else:
                        raise ValueError("没有可用的 IMAP 邮箱服务，请先在邮箱服务中添加")
                elif service_type == EmailServiceType.LUCKMAIL:
                    from ...database.models import EmailService as EmailServiceModel

                    db_service = db.query(EmailServiceModel).filter(
                        EmailServiceModel.service_type == "luckmail",
                        EmailServiceModel.enabled == True
                    ).order_by(EmailServiceModel.priority.asc()).first()

                    if db_service and db_service.config:
                        config = _normalize_email_service_config(service_type, db_service.config, actual_proxy_url)
                        _safe_update_registration_task(db, task_uuid, context="绑定 luckmail 服务", email_service_id=db_service.id)
                        logger.info(f"使用数据库 LuckMail 服务: {db_service.name}")
                    else:
                        config = _normalize_email_service_config(service_type, email_service_config or {}, actual_proxy_url)
                        if not config.get("api_key"):
                            raise ValueError("没有可用的 LuckMail 服务，请先在邮箱服务中添加并填写 API Key")
                else:
                    config = email_service_config or {}

            email_service = EmailServiceFactory.create(service_type, config)

            # 创建注册引擎 - 同时把日志写入任务通道和批量任务兜底快照
            log_callback = _create_registration_log_callback(task_uuid, log_prefix=log_prefix, batch_id=batch_id)

            def raise_if_cancelled():
                task_manager.wait_if_paused(task_uuid)
                if task_manager.is_cancelled(task_uuid):
                    raise RegistrationCancelledError("任务已取消，停止继续执行")

            engine_kwargs = {
                "email_service": email_service,
                "proxy_url": actual_proxy_url,
                "callback_logger": log_callback,
                "task_uuid": task_uuid,
                "check_cancelled": raise_if_cancelled,
            }
            extra_config = {}
            if requested_token_completion_concurrency > 1 or token_completion_max_concurrency > 0:
                extra_config.update({
                    "token_completion_concurrency": requested_token_completion_concurrency,
                    "token_completion_max_concurrency": token_completion_max_concurrency,
                })
            if chatgpt_registration_mode:
                extra_config["chatgpt_registration_mode"] = str(chatgpt_registration_mode).strip()
            if extra_config:
                engine_kwargs["extra_config"] = extra_config

            engine = RegistrationEngine(
                **engine_kwargs,
            )

            # 执行注册
            result = engine.run()
            if task_manager.is_cancelled(task_uuid):
                cancel_reason = getattr(result, "error_message", None) or "任务已取消"
                _finalize_task_record(
                    db,
                    task_uuid,
                    status="cancelled",
                    error_message=cancel_reason,
                )
                task_manager.update_status(task_uuid, "cancelled")
                logger.info(f"注册任务取消收尾完成: {task_uuid}")
                return
            marker = getattr(email_service, "mark_registration_outcome", None)
            marker_context = {}
            try:
                info = getattr(engine, "email_info", None) or {}
                for key in ("service_id", "order_no", "token", "purchase_id", "source"):
                    value = info.get(key) if isinstance(info, dict) else None
                    if value not in (None, ""):
                        marker_context[key] = value
            except Exception:
                marker_context = {}

            if result.success:
                # 更新代理使用时间
                update_proxy_usage(db, proxy_id)

                # 保存到数据库
                saved_to_database = engine.save_to_database(result)
                if not saved_to_database:
                    save_error = "保存到数据库失败，任务未标记完成"
                    _finalize_task_record(
                        db,
                        task_uuid,
                        status="failed",
                        error_message=save_error,
                    )
                    task_manager.update_status(task_uuid, "failed", error=save_error)
                    logger.error(f"注册任务落库失败: {task_uuid}, 邮箱: {result.email}")
                    return

                if callable(marker) and result.email:
                    try:
                        marker(
                            email=result.email,
                            success=True,
                            context=marker_context,
                        )
                    except Exception as mark_err:
                        logger.warning(f"记录邮箱成功状态失败: {mark_err}")

                # 自动上传到 CPA（可多服务）
                if auto_upload_cpa:
                    try:
                        from ...core.upload.cpa_upload import upload_to_cpa, generate_token_json
                        from ...database.models import Account as AccountModel
                        saved_account = db.query(AccountModel).filter_by(email=result.email).first()
                        if saved_account and saved_account.access_token:
                            token_data = generate_token_json(saved_account)
                            _cpa_ids = cpa_service_ids or []
                            if not _cpa_ids:
                                # 未指定则取所有启用的服务
                                _cpa_ids = [s.id for s in crud.get_cpa_services(db, enabled=True)]
                            if not _cpa_ids:
                                log_callback("[CPA] 无可用 CPA 服务，跳过上传")
                            for _sid in _cpa_ids:
                                try:
                                    _svc = crud.get_cpa_service_by_id(db, _sid)
                                    if not _svc:
                                        continue
                                    log_callback(f"[CPA] 正在把账号打包发往服务站: {_svc.name}")
                                    _ok, _msg = upload_to_cpa(token_data, api_url=_svc.api_url, api_token=_svc.api_token)
                                    if _ok:
                                        saved_account.cpa_uploaded = True
                                        saved_account.cpa_uploaded_at = datetime.utcnow()
                                        db.commit()
                                        log_callback(f"[CPA] 投递成功，服务站已签收: {_svc.name}")
                                    else:
                                        log_callback(f"[CPA] 上传失败({_svc.name}): {_msg}")
                                except Exception as _e:
                                    log_callback(f"[CPA] 异常({_sid}): {_e}")
                    except Exception as cpa_err:
                        log_callback(f"[CPA] 上传异常: {cpa_err}")

                # 自动上传到 Sub2API（可多服务）
                if auto_upload_sub2api:
                    try:
                        from ...core.upload.sub2api_upload import upload_to_sub2api
                        from ...database.models import Account as AccountModel
                        saved_account = db.query(AccountModel).filter_by(email=result.email).first()
                        if saved_account and saved_account.access_token:
                            _s2a_ids = sub2api_service_ids or []
                            if not _s2a_ids:
                                _s2a_ids = [s.id for s in crud.get_sub2api_services(db, enabled=True)]
                            if not _s2a_ids:
                                log_callback("[Sub2API] 无可用 Sub2API 服务，跳过上传")
                            for _sid in _s2a_ids:
                                try:
                                    _svc = crud.get_sub2api_service_by_id(db, _sid)
                                    if not _svc:
                                        continue
                                    log_callback(f"[Sub2API] 正在把账号发往服务站: {_svc.name}")
                                    _ok, _msg = upload_to_sub2api([saved_account], _svc.api_url, _svc.api_key)
                                    if _ok:
                                        saved_account.sub2api_uploaded = True
                                        saved_account.sub2api_uploaded_at = datetime.utcnow()
                                        db.commit()
                                    log_callback(f"[Sub2API] {'成功' if _ok else '失败'}({_svc.name}): {_msg}")
                                except Exception as _e:
                                    log_callback(f"[Sub2API] 异常({_sid}): {_e}")
                    except Exception as s2a_err:
                        log_callback(f"[Sub2API] 上传异常: {s2a_err}")

                # 自动上传到 Team Manager（可多服务）
                if auto_upload_tm:
                    try:
                        from ...core.upload.team_manager_upload import upload_to_team_manager
                        from ...database.models import Account as AccountModel
                        saved_account = db.query(AccountModel).filter_by(email=result.email).first()
                        if saved_account and saved_account.access_token:
                            _tm_ids = tm_service_ids or []
                            if not _tm_ids:
                                _tm_ids = [s.id for s in crud.get_tm_services(db, enabled=True)]
                            if not _tm_ids:
                                log_callback("[TM] 无可用 Team Manager 服务，跳过上传")
                            for _sid in _tm_ids:
                                try:
                                    _svc = crud.get_tm_service_by_id(db, _sid)
                                    if not _svc:
                                        continue
                                    log_callback(f"[TM] 正在把账号发往服务站: {_svc.name}")
                                    _ok, _msg = upload_to_team_manager(saved_account, _svc.api_url, _svc.api_key)
                                    log_callback(f"[TM] {'成功' if _ok else '失败'}({_svc.name}): {_msg}")
                                except Exception as _e:
                                    log_callback(f"[TM] 异常({_sid}): {_e}")
                    except Exception as tm_err:
                        log_callback(f"[TM] 上传异常: {tm_err}")

                # 更新任务状态
                _finalize_task_record(
                    db,
                    task_uuid,
                    status="completed",
                    result=result.to_dict(),
                )

                # 更新 TaskManager 状态
                task_manager.update_status(task_uuid, "completed", email=result.email)

                logger.info(f"注册任务完成: {task_uuid}, 邮箱: {result.email}")
            else:
                if callable(marker) and result.email:
                    try:
                        marker(
                            email=result.email,
                            success=False,
                            reason=result.error_message or "",
                            context=marker_context,
                        )
                    except Exception as mark_err:
                        logger.warning(f"记录邮箱失败状态失败: {mark_err}")

                # 更新任务状态为失败
                _finalize_task_record(
                    db,
                    task_uuid,
                    status="failed",
                    error_message=result.error_message,
                )

                # 更新 TaskManager 状态
                task_manager.update_status(task_uuid, "failed", error=result.error_message)

                logger.warning(f"注册任务失败: {task_uuid}, 原因: {result.error_message}")

        except RegistrationCancelledError as e:
            _finalize_task_record(
                db,
                task_uuid,
                status="cancelled",
                error_message=str(e),
            )
            task_manager.update_status(task_uuid, "cancelled")
            logger.info(f"注册任务取消完成: {task_uuid}")
        except Exception as e:
            logger.error(f"注册任务异常: {task_uuid}, 错误: {e}")

            try:
                with get_db() as db:
                    crud.update_registration_task(
                        db,
                        task_uuid,
                        status="failed",
                        completed_at=datetime.utcnow(),
                        error_message=str(e),
                        logs=_snapshot_runtime_logs(task_uuid),
                    )

                # 更新 TaskManager 状态
                task_manager.update_status(task_uuid, "failed", error=str(e))
            except:
                pass


async def run_registration_task(task_uuid: str, email_service_type: str, proxy: Optional[str], email_service_config: Optional[dict], email_service_id: Optional[int] = None, log_prefix: str = "", batch_id: str = "", auto_upload_cpa: bool = False, cpa_service_ids: List[int] = None, auto_upload_sub2api: bool = False, sub2api_service_ids: List[int] = None, auto_upload_tm: bool = False, tm_service_ids: List[int] = None, token_completion_concurrency: int = 1, chatgpt_registration_mode: str = "refresh_token"):
    """
    异步执行注册任务

    使用 run_in_executor 将同步任务放入线程池执行，避免阻塞主事件循环
    """
    loop = task_manager.get_loop()
    if loop is None:
        loop = asyncio.get_event_loop()
        task_manager.set_loop(loop)

    # 初始化 TaskManager 状态
    task_manager.update_status(task_uuid, "pending")
    task_manager.add_log(task_uuid, f"{log_prefix} [系统] 任务 {task_uuid[:8]} 已加入队列" if log_prefix else f"[系统] 任务 {task_uuid[:8]} 已加入队列")

    try:
        # 在线程池中执行同步任务（传入 log_prefix 和 batch_id 供回调使用）
        await loop.run_in_executor(
            task_manager.executor,
            _run_sync_registration_task,
            task_uuid,
            email_service_type,
            proxy,
            email_service_config,
            email_service_id,
            log_prefix,
            batch_id,
            auto_upload_cpa,
            cpa_service_ids or [],
            auto_upload_sub2api,
            sub2api_service_ids or [],
            auto_upload_tm,
            tm_service_ids or [],
            token_completion_concurrency,
            chatgpt_registration_mode,
        )
    except Exception as e:
        logger.error(f"线程池执行异常: {task_uuid}, 错误: {e}")
        task_manager.add_log(task_uuid, f"[错误] 线程池执行异常: {str(e)}")
        task_manager.update_status(task_uuid, "failed", error=str(e))


def _init_batch_state(batch_id: str, task_uuids: List[str]):
    """初始化批量任务内存状态"""
    existing = batch_tasks.get(batch_id, {})
    preserved_cancelled = bool(existing.get("cancelled"))
    preserved_paused = bool(existing.get("paused"))
    preserved_logs = list(existing.get("logs", []))
    preserved_skipped = int(existing.get("skipped", 0) or 0)
    task_manager.init_batch(batch_id, len(task_uuids))
    task_manager.bind_batch_tasks(batch_id, task_uuids)
    batch_tasks[batch_id] = {
        "status": "paused" if preserved_paused else "running",
        "total": len(task_uuids),
        "completed": 0,
        "success": 0,
        "failed": 0,
        "cancelled": preserved_cancelled,
        "paused": preserved_paused,
        "task_uuids": task_uuids,
        "current_index": 0,
        "logs": preserved_logs,
        "skipped": preserved_skipped,
        "finished": False
    }
    if preserved_cancelled:
        task_manager.cancel_batch(batch_id)
    elif preserved_paused:
        task_manager.pause_batch(batch_id)
        with get_db() as db:
            _update_batch_child_task_statuses(db, batch_id, action="pause")


def _append_persisted_batch_log(batch_id: str, log_message: str):
    """把批量日志同步写入 batch_tasks 快照，作为实时日志链路失效时的兜底。"""
    if not batch_id or batch_id not in batch_tasks:
        return
    logs = batch_tasks[batch_id].setdefault("logs", [])
    logs.append(log_message)
    overflow = len(logs) - _BATCH_LOG_HISTORY_LIMIT
    if overflow > 0:
        del logs[:overflow]


def _create_registration_log_callback(task_uuid: str, log_prefix: str = "", batch_id: str = ""):
    """创建注册日志回调，并把批量日志同步沉淀到批量快照。"""
    def callback(msg: str):
        full_msg = f"{log_prefix} {msg}" if log_prefix else msg
        task_manager.add_log(task_uuid, full_msg)
        if batch_id:
            _append_persisted_batch_log(batch_id, full_msg)
            task_manager.add_batch_log(batch_id, full_msg)

    return callback


def _resolve_batch_token_completion_concurrency(
    *,
    batch_concurrency: int,
    requested_token_completion_concurrency: Optional[int],
    settings,
) -> int:
    """解析批量任务中的 token completion 并发。"""
    resolved_batch_concurrency = max(1, int(batch_concurrency or 1))
    token_completion_max_concurrency = max(
        0,
        int(getattr(settings, "registration_token_completion_max_concurrency", 0) or 0),
    )

    if requested_token_completion_concurrency is not None:
        resolved = max(1, int(requested_token_completion_concurrency or 1))
        if token_completion_max_concurrency > 0:
            resolved = min(resolved, token_completion_max_concurrency)
        return resolved

    if token_completion_max_concurrency > 0:
        return min(resolved_batch_concurrency, token_completion_max_concurrency)

    # 未显式配置时采用保守自动值，避免在高并发批量下无感知退成 1。
    return min(resolved_batch_concurrency, 2)


def _make_batch_helpers(batch_id: str):
    """返回 add_batch_log 和 update_batch_status 辅助函数"""
    def add_batch_log(msg: str):
        _append_persisted_batch_log(batch_id, msg)
        task_manager.add_batch_log(batch_id, msg)

    def update_batch_status(**kwargs):
        for key, value in kwargs.items():
            if key in batch_tasks[batch_id]:
                batch_tasks[batch_id][key] = value
        task_manager.update_batch_status(batch_id, **kwargs)

    return add_batch_log, update_batch_status


async def run_batch_parallel(
    batch_id: str,
    task_uuids: List[str],
    email_service_type: str,
    proxy: Optional[str],
    email_service_config: Optional[dict],
    email_service_id: Optional[int],
    concurrency: int,
    auto_upload_cpa: bool = False,
    cpa_service_ids: List[int] = None,
    auto_upload_sub2api: bool = False,
    sub2api_service_ids: List[int] = None,
    auto_upload_tm: bool = False,
    tm_service_ids: List[int] = None,
    token_completion_concurrency: Optional[int] = None,
    chatgpt_registration_mode: str = "refresh_token",
):
    """
    并行模式：所有任务同时提交，Semaphore 控制最大并发数
    """
    _init_batch_state(batch_id, task_uuids)
    add_batch_log, update_batch_status = _make_batch_helpers(batch_id)
    semaphore = asyncio.Semaphore(concurrency)
    counter_lock = asyncio.Lock()
    resolved_token_completion_concurrency = _resolve_batch_token_completion_concurrency(
        batch_concurrency=concurrency,
        requested_token_completion_concurrency=token_completion_concurrency,
        settings=get_settings(),
    )
    add_batch_log(f"[系统] 并行模式启动，并发数: {concurrency}，总任务: {len(task_uuids)}")
    add_batch_log(f"[系统] Token 收尾并发: {resolved_token_completion_concurrency}")

    async def _run_one(idx: int, uuid: str):
        prefix = f"[任务{idx + 1}]"
        if task_manager.is_batch_cancelled(batch_id):
            with get_db() as db:
                _finalize_task_record(db, uuid, status="cancelled", error_message="批量任务已取消")
            return
        await _wait_if_batch_paused(batch_id)
        async with semaphore:
            await _wait_if_batch_paused(batch_id)
            if task_manager.is_batch_cancelled(batch_id):
                with get_db() as db:
                    _finalize_task_record(db, uuid, status="cancelled", error_message="批量任务已取消")
                return
            add_batch_log(f"{prefix} 开始注册...")
            await run_registration_task(
                uuid, email_service_type, proxy, email_service_config, email_service_id,
                log_prefix=prefix, batch_id=batch_id,
                auto_upload_cpa=auto_upload_cpa, cpa_service_ids=cpa_service_ids or [],
                auto_upload_sub2api=auto_upload_sub2api, sub2api_service_ids=sub2api_service_ids or [],
                auto_upload_tm=auto_upload_tm, tm_service_ids=tm_service_ids or [],
                token_completion_concurrency=resolved_token_completion_concurrency,
                chatgpt_registration_mode=chatgpt_registration_mode,
            )
        with get_db() as db:
            t = crud.get_registration_task(db, uuid)
            if t:
                async with counter_lock:
                    new_completed = batch_tasks[batch_id]["completed"] + 1
                    new_success = batch_tasks[batch_id]["success"]
                    new_failed = batch_tasks[batch_id]["failed"]
                    if t.status == "completed":
                        new_success += 1
                        add_batch_log(f"{prefix} [成功] 注册成功")
                    elif t.status == "failed":
                        new_failed += 1
                        add_batch_log(f"{prefix} [失败] 注册失败: {t.error_message}")
                    update_batch_status(completed=new_completed, success=new_success, failed=new_failed)

    try:
        await asyncio.gather(*[_run_one(i, u) for i, u in enumerate(task_uuids)], return_exceptions=True)
        if not task_manager.is_batch_cancelled(batch_id):
            add_batch_log(f"[完成] 批量任务完成！成功: {batch_tasks[batch_id]['success']}, 失败: {batch_tasks[batch_id]['failed']}")
            update_batch_status(finished=True, status="completed")
        else:
            update_batch_status(finished=True, status="cancelled")
    except Exception as e:
        logger.error(f"批量任务 {batch_id} 异常: {e}")
        add_batch_log(f"[错误] 批量任务异常: {str(e)}")
        update_batch_status(finished=True, status="failed")
    finally:
        batch_tasks[batch_id]["finished"] = True


async def run_batch_pipeline(
    batch_id: str,
    task_uuids: List[str],
    email_service_type: str,
    proxy: Optional[str],
    email_service_config: Optional[dict],
    email_service_id: Optional[int],
    interval_min: int,
    interval_max: int,
    concurrency: int,
    auto_upload_cpa: bool = False,
    cpa_service_ids: List[int] = None,
    auto_upload_sub2api: bool = False,
    sub2api_service_ids: List[int] = None,
    auto_upload_tm: bool = False,
    tm_service_ids: List[int] = None,
    token_completion_concurrency: Optional[int] = None,
    chatgpt_registration_mode: str = "refresh_token",
):
    """
    流水线模式：每隔 interval 秒启动一个新任务，Semaphore 限制最大并发数
    """
    _init_batch_state(batch_id, task_uuids)
    add_batch_log, update_batch_status = _make_batch_helpers(batch_id)
    semaphore = asyncio.Semaphore(concurrency)
    counter_lock = asyncio.Lock()
    running_tasks_list = []
    resolved_token_completion_concurrency = _resolve_batch_token_completion_concurrency(
        batch_concurrency=concurrency,
        requested_token_completion_concurrency=token_completion_concurrency,
        settings=get_settings(),
    )
    add_batch_log(f"[系统] 流水线模式启动，并发数: {concurrency}，总任务: {len(task_uuids)}")
    add_batch_log(f"[系统] Token 收尾并发: {resolved_token_completion_concurrency}")

    async def _run_and_release(idx: int, uuid: str, pfx: str):
        try:
            await run_registration_task(
                uuid, email_service_type, proxy, email_service_config, email_service_id,
                log_prefix=pfx, batch_id=batch_id,
                auto_upload_cpa=auto_upload_cpa, cpa_service_ids=cpa_service_ids or [],
                auto_upload_sub2api=auto_upload_sub2api, sub2api_service_ids=sub2api_service_ids or [],
                auto_upload_tm=auto_upload_tm, tm_service_ids=tm_service_ids or [],
                token_completion_concurrency=resolved_token_completion_concurrency,
                chatgpt_registration_mode=chatgpt_registration_mode,
            )
            with get_db() as db:
                t = crud.get_registration_task(db, uuid)
                if t:
                    async with counter_lock:
                        new_completed = batch_tasks[batch_id]["completed"] + 1
                        new_success = batch_tasks[batch_id]["success"]
                        new_failed = batch_tasks[batch_id]["failed"]
                        if t.status == "completed":
                            new_success += 1
                            add_batch_log(f"{pfx} [成功] 注册成功")
                        elif t.status == "failed":
                            new_failed += 1
                            add_batch_log(f"{pfx} [失败] 注册失败: {t.error_message}")
                        update_batch_status(completed=new_completed, success=new_success, failed=new_failed)
        finally:
            semaphore.release()

    try:
        for i, task_uuid in enumerate(task_uuids):
            await _wait_if_batch_paused(batch_id)
            if task_manager.is_batch_cancelled(batch_id) or batch_tasks[batch_id]["cancelled"]:
                with get_db() as db:
                    for remaining_uuid in task_uuids[i:]:
                        task_manager.cancel_task(remaining_uuid)
                        _finalize_task_record(db, remaining_uuid, status="cancelled", error_message="批量任务已取消")
                add_batch_log("[取消] 批量任务已取消")
                update_batch_status(finished=True, status="cancelled")
                break

            update_batch_status(current_index=i)
            await semaphore.acquire()
            prefix = f"[任务{i + 1}]"
            add_batch_log(f"{prefix} 开始注册...")
            t = asyncio.create_task(_run_and_release(i, task_uuid, prefix))
            running_tasks_list.append(t)

            if i < len(task_uuids) - 1 and not task_manager.is_batch_cancelled(batch_id):
                wait_time = random.randint(interval_min, interval_max)
                logger.info(f"批量任务 {batch_id}: 等待 {wait_time} 秒后启动下一个任务")
                await _sleep_with_batch_controls(batch_id, wait_time)

        if running_tasks_list:
            await asyncio.gather(*running_tasks_list, return_exceptions=True)

        if not task_manager.is_batch_cancelled(batch_id):
            add_batch_log(f"[完成] 批量任务完成！成功: {batch_tasks[batch_id]['success']}, 失败: {batch_tasks[batch_id]['failed']}")
            update_batch_status(finished=True, status="completed")
    except Exception as e:
        logger.error(f"批量任务 {batch_id} 异常: {e}")
        add_batch_log(f"[错误] 批量任务异常: {str(e)}")
        update_batch_status(finished=True, status="failed")
    finally:
        batch_tasks[batch_id]["finished"] = True


async def run_batch_registration(
    batch_id: str,
    task_uuids: List[str],
    email_service_type: str,
    proxy: Optional[str],
    email_service_config: Optional[dict],
    email_service_id: Optional[int],
    interval_min: int,
    interval_max: int,
    concurrency: int = 1,
    mode: str = "pipeline",
    auto_upload_cpa: bool = False,
    cpa_service_ids: List[int] = None,
    auto_upload_sub2api: bool = False,
    sub2api_service_ids: List[int] = None,
    auto_upload_tm: bool = False,
    tm_service_ids: List[int] = None,
    token_completion_concurrency: Optional[int] = None,
    chatgpt_registration_mode: str = "refresh_token",
):
    """根据 mode 分发到并行或流水线执行"""
    if mode == "parallel":
        await run_batch_parallel(
            batch_id, task_uuids, email_service_type, proxy,
            email_service_config, email_service_id, concurrency,
            auto_upload_cpa=auto_upload_cpa, cpa_service_ids=cpa_service_ids,
            auto_upload_sub2api=auto_upload_sub2api, sub2api_service_ids=sub2api_service_ids,
            auto_upload_tm=auto_upload_tm, tm_service_ids=tm_service_ids,
            token_completion_concurrency=token_completion_concurrency,
            chatgpt_registration_mode=chatgpt_registration_mode,
        )
    else:
        await run_batch_pipeline(
            batch_id, task_uuids, email_service_type, proxy,
            email_service_config, email_service_id,
            interval_min, interval_max, concurrency,
            auto_upload_cpa=auto_upload_cpa, cpa_service_ids=cpa_service_ids,
            auto_upload_sub2api=auto_upload_sub2api, sub2api_service_ids=sub2api_service_ids,
            auto_upload_tm=auto_upload_tm, tm_service_ids=tm_service_ids,
            token_completion_concurrency=token_completion_concurrency,
            chatgpt_registration_mode=chatgpt_registration_mode,
        )


# ============== API Endpoints ==============

@router.post("/start", response_model=RegistrationTaskResponse)
async def start_registration(
    request: RegistrationTaskCreate,
    background_tasks: BackgroundTasks
):
    """
    启动注册任务

    - email_service_type: 邮箱服务类型 (tempmail, outlook, moe_mail)
    - proxy: 代理地址
    - email_service_config: 邮箱服务配置（outlook 需要提供账户信息）
    """
    # 验证邮箱服务类型
    try:
        EmailServiceType(request.email_service_type)
    except ValueError:
        raise HTTPException(
            status_code=400,
            detail=f"无效的邮箱服务类型: {request.email_service_type}"
        )

    # 创建任务
    task_uuid = str(uuid.uuid4())

    with get_db() as db:
        task = crud.create_registration_task(
            db,
            task_uuid=task_uuid,
            proxy=request.proxy
        )

    # 在后台运行注册任务
    background_tasks.add_task(
        run_registration_task,
        task_uuid,
        request.email_service_type,
        request.proxy,
        request.email_service_config,
        request.email_service_id,
        log_prefix="",
        batch_id="",
        auto_upload_cpa=request.auto_upload_cpa,
        cpa_service_ids=request.cpa_service_ids,
        auto_upload_sub2api=request.auto_upload_sub2api,
        sub2api_service_ids=request.sub2api_service_ids,
        auto_upload_tm=request.auto_upload_tm,
        tm_service_ids=request.tm_service_ids,
        chatgpt_registration_mode=request.chatgpt_registration_mode,
    )

    return task_to_response(task)


@router.post("/batch", response_model=BatchRegistrationResponse)
async def start_batch_registration(
    request: BatchRegistrationRequest,
    background_tasks: BackgroundTasks
):
    """
    启动批量注册任务

    - count: 注册数量 (1-1000)
    - email_service_type: 邮箱服务类型
    - proxy: 代理地址
    - interval_min: 最小间隔秒数
    - interval_max: 最大间隔秒数
    """
    # 验证参数
    if request.count < 1 or request.count > 1000:
        raise HTTPException(status_code=400, detail="注册数量必须在 1-1000 之间")

    try:
        EmailServiceType(request.email_service_type)
    except ValueError:
        raise HTTPException(
            status_code=400,
            detail=f"无效的邮箱服务类型: {request.email_service_type}"
        )

    if request.interval_min < 0 or request.interval_max < request.interval_min:
        raise HTTPException(status_code=400, detail="间隔时间参数无效")

    if not 1 <= request.concurrency <= 50:
        raise HTTPException(status_code=400, detail="并发数必须在 1-50 之间")

    if request.token_completion_concurrency is not None and not 1 <= request.token_completion_concurrency <= 50:
        raise HTTPException(status_code=400, detail="Token 收尾并发必须在 1-50 之间")

    if request.mode not in ("parallel", "pipeline"):
        raise HTTPException(status_code=400, detail="模式必须为 parallel 或 pipeline")

    # 创建批量任务
    batch_id = str(uuid.uuid4())
    task_uuids = []

    with get_db() as db:
        for _ in range(request.count):
            task_uuid = str(uuid.uuid4())
            task = crud.create_registration_task(
                db,
                task_uuid=task_uuid,
                proxy=request.proxy
            )
            task_uuids.append(task_uuid)

    # 获取所有任务
    with get_db() as db:
        tasks = [crud.get_registration_task(db, uuid) for uuid in task_uuids]

    batch_tasks[batch_id] = {
        "status": "running",
        "total": len(task_uuids),
        "completed": 0,
        "success": 0,
        "failed": 0,
        "skipped": 0,
        "cancelled": False,
        "paused": False,
        "task_uuids": task_uuids,
        "current_index": 0,
        "logs": [],
        "finished": False,
    }
    task_manager.bind_batch_tasks(batch_id, task_uuids)

    # 在后台运行批量注册
    background_tasks.add_task(
        run_batch_registration,
        batch_id,
        task_uuids,
        request.email_service_type,
        request.proxy,
        request.email_service_config,
        request.email_service_id,
        request.interval_min,
        request.interval_max,
        concurrency=request.concurrency,
        mode=request.mode,
        auto_upload_cpa=request.auto_upload_cpa,
        cpa_service_ids=request.cpa_service_ids,
        auto_upload_sub2api=request.auto_upload_sub2api,
        sub2api_service_ids=request.sub2api_service_ids,
        auto_upload_tm=request.auto_upload_tm,
        tm_service_ids=request.tm_service_ids,
        token_completion_concurrency=request.token_completion_concurrency,
        chatgpt_registration_mode=request.chatgpt_registration_mode,
    )

    return BatchRegistrationResponse(
        batch_id=batch_id,
        count=request.count,
        tasks=[task_to_response(t) for t in tasks if t]
    )


@router.get("/batch/{batch_id}")
async def get_batch_status(batch_id: str, log_offset: int = Query(0, ge=0)):
    """获取批量任务状态"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    runtime_logs = task_manager.get_batch_logs(batch_id)
    log_base_index = task_manager.get_batch_log_base_index(batch_id) if runtime_logs else 0
    resolved_logs = runtime_logs if runtime_logs else batch.get("logs", [])
    incremental_logs, normalized_offset, log_next_offset = _resolve_log_window(
        resolved_logs,
        offset=log_offset,
        base_index=log_base_index,
    )
    return {
        "batch_id": batch_id,
        "status": batch.get("status", "running"),
        "total": batch["total"],
        "completed": batch["completed"],
        "success": batch["success"],
        "failed": batch["failed"],
        "current_index": batch["current_index"],
        "cancelled": batch["cancelled"],
        "paused": batch.get("paused", False),
        "finished": batch.get("finished", False),
        "logs": incremental_logs,
        "log_base_index": log_base_index,
        "log_offset": normalized_offset,
        "log_next_offset": log_next_offset,
        "progress": f"{batch['completed']}/{batch['total']}"
    }


@router.post("/batch/{batch_id}/pause")
async def pause_batch(batch_id: str):
    """暂停批量任务。"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    if batch.get("finished"):
        raise HTTPException(status_code=400, detail="批量任务已完成")
    if batch.get("paused"):
        raise HTTPException(status_code=400, detail="批量任务已暂停")

    batch["paused"] = True
    batch["status"] = "paused"
    task_manager.pause_batch(batch_id)
    task_manager.update_batch_status(batch_id, paused=True, status="paused")
    with get_db() as db:
        _update_batch_child_task_statuses(db, batch_id, action="pause")
    return {"success": True, "status": "paused", "message": "批量任务已暂停"}


@router.post("/batch/{batch_id}/resume")
async def resume_batch(batch_id: str):
    """恢复批量任务。"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    if batch.get("finished"):
        raise HTTPException(status_code=400, detail="批量任务已完成")
    if not batch.get("paused"):
        raise HTTPException(status_code=400, detail="批量任务未暂停")

    batch["paused"] = False
    batch["status"] = "running"
    task_manager.resume_batch(batch_id)
    task_manager.update_batch_status(batch_id, paused=False, status="running")
    with get_db() as db:
        _update_batch_child_task_statuses(db, batch_id, action="resume")
    return {"success": True, "status": "running", "message": "批量任务已恢复"}


@router.post("/batch/{batch_id}/cancel")
async def cancel_batch(batch_id: str):
    """取消批量任务"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    if batch.get("finished"):
        raise HTTPException(status_code=400, detail="批量任务已完成")

    batch["cancelled"] = True
    batch["paused"] = False
    batch["status"] = "cancelling"
    task_manager.bind_batch_tasks(batch_id, batch.get("task_uuids", []))
    task_manager.cancel_batch(batch_id)
    return {"success": True, "message": "批量任务取消请求已提交，正在让它们有序收工"}


@router.get("/tasks", response_model=TaskListResponse)
async def list_tasks(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    status: Optional[str] = Query(None),
):
    """获取任务列表"""
    with get_db() as db:
        query = db.query(RegistrationTask)

        if status:
            query = query.filter(RegistrationTask.status == status)

        total = query.count()
        offset = (page - 1) * page_size
        tasks = query.order_by(RegistrationTask.created_at.desc()).offset(offset).limit(page_size).all()

        return TaskListResponse(
            total=total,
            tasks=[task_to_response(t) for t in tasks]
        )


@router.get("/tasks/{task_uuid}", response_model=RegistrationTaskResponse)
async def get_task(task_uuid: str):
    """获取任务详情"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")
        return task_to_response(task)


@router.get("/tasks/{task_uuid}/logs")
async def get_task_logs(task_uuid: str, offset: int = Query(0, ge=0)):
    """获取任务日志"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")

        logs = task.logs or ""
        runtime_logs = task_manager.get_logs(task_uuid)
        resolved_logs = runtime_logs if runtime_logs else (logs.split("\n") if logs else [])
        incremental_logs, normalized_offset, log_next_offset = _resolve_log_window(
            resolved_logs,
            offset=offset,
        )
        result = task.result if isinstance(task.result, dict) else {}
        email = result.get("email")
        service_type = task.email_service.service_type if task.email_service else None
        return {
            "task_uuid": task_uuid,
            "status": task.status,
            "email": email,
            "email_service": service_type,
            "logs": incremental_logs,
            "log_offset": normalized_offset,
            "log_next_offset": log_next_offset,
        }


@router.post("/tasks/{task_uuid}/pause")
async def pause_task(task_uuid: str):
    """暂停任务。"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")
        if task.status not in ["pending", "running"]:
            raise HTTPException(status_code=400, detail="当前状态不支持暂停")

        status = _pause_single_task_record(db, task)
        return {"success": True, "status": status, "message": "任务已暂停"}


@router.post("/tasks/{task_uuid}/resume")
async def resume_task(task_uuid: str):
    """恢复任务。"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")
        if task.status != "paused":
            raise HTTPException(status_code=400, detail="任务未暂停")

        status = _resume_single_task_record(db, task)
        return {"success": True, "status": status, "message": "任务已恢复"}


@router.post("/tasks/{task_uuid}/cancel")
async def cancel_task(task_uuid: str):
    """取消任务"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")

        if task.status not in ["pending", "running", "paused"]:
            raise HTTPException(status_code=400, detail="任务已完成或已取消")

        task_manager.cancel_task(task_uuid)
        task_manager.update_status(task_uuid, "cancelling")
        task = crud.update_registration_task(db, task_uuid, status="cancelling")

        return {"success": True, "message": "任务取消请求已提交，正在等待当前步骤收尾"}


@router.delete("/tasks/{task_uuid}")
async def delete_task(task_uuid: str):
    """删除任务"""
    with get_db() as db:
        task = crud.get_registration_task(db, task_uuid)
        if not task:
            raise HTTPException(status_code=404, detail="任务不存在")

        if task.status == "running":
            raise HTTPException(status_code=400, detail="无法删除运行中的任务")

        crud.delete_registration_task(db, task_uuid)

        return {"success": True, "message": "任务已删除"}


@router.get("/stats")
async def get_registration_stats():
    """获取注册统计信息"""
    with get_db() as db:
        from sqlalchemy import func

        # 按状态统计
        status_stats = db.query(
            RegistrationTask.status,
            func.count(RegistrationTask.id)
        ).group_by(RegistrationTask.status).all()

        # 今日统计
        today = datetime.utcnow().date()
        today_status_stats = db.query(
            RegistrationTask.status,
            func.count(RegistrationTask.id)
        ).filter(
            func.date(RegistrationTask.created_at) == today
        ).group_by(RegistrationTask.status).all()

        today_count = db.query(func.count(RegistrationTask.id)).filter(
            func.date(RegistrationTask.created_at) == today
        ).scalar()

        today_by_status = {status: count for status, count in today_status_stats}
        today_success = int(today_by_status.get("completed", 0))
        today_failed = int(today_by_status.get("failed", 0))
        today_total = int(today_count or 0)
        today_success_rate = round((today_success / today_total) * 100, 1) if today_total > 0 else 0.0

        return {
            "by_status": {status: count for status, count in status_stats},
            "today_count": today_total,
            "today_total": today_total,
            "today_success": today_success,
            "today_failed": today_failed,
            "today_success_rate": today_success_rate,
            "today_by_status": today_by_status,
        }


@router.get("/available-services")
async def get_available_email_services():
    """
    获取可用于注册的邮箱服务列表

    返回所有已启用的邮箱服务，包括：
    - tempmail: 临时邮箱（无需配置）
    - yyds_mail: YYDS Mail 临时邮箱（需 API Key）
    - outlook: 已导入的 Outlook 账户
    - moe_mail: 已配置的自定义域名服务
    """
    from ...database.models import EmailService as EmailServiceModel
    from ...config.settings import get_settings

    settings = get_settings()
    result = {
        "tempmail": {
            "available": bool(settings.tempmail_enabled),
            "count": 1 if settings.tempmail_enabled else 0,
            "services": ([{
                "id": None,
                "name": "Tempmail.lol",
                "type": "tempmail",
                "description": "临时邮箱，自动创建"
            }] if settings.tempmail_enabled else [])
        },
        "yyds_mail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "outlook": {
            "available": False,
            "count": 0,
            "services": []
        },
        "moe_mail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "temp_mail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "duck_mail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "freemail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "imap_mail": {
            "available": False,
            "count": 0,
            "services": []
        },
        "luckmail": {
            "available": False,
            "count": 0,
            "services": []
        }
    }

    yyds_api_key = settings.yyds_mail_api_key.get_secret_value() if settings.yyds_mail_api_key else ""
    if settings.yyds_mail_enabled and yyds_api_key:
        result["yyds_mail"]["available"] = True
        result["yyds_mail"]["count"] = 1
        result["yyds_mail"]["services"].append({
            "id": None,
            "name": "YYDS Mail",
            "type": "yyds_mail",
            "default_domain": settings.yyds_mail_default_domain or None,
            "description": "YYDS Mail API 临时邮箱",
        })

    with get_db() as db:
        yyds_mail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "yyds_mail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in yyds_mail_services:
            config = service.config or {}
            result["yyds_mail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "yyds_mail",
                "default_domain": config.get("default_domain"),
                "priority": service.priority
            })

        if yyds_mail_services:
            result["yyds_mail"]["count"] = len(result["yyds_mail"]["services"])
            result["yyds_mail"]["available"] = True
        # 获取 Outlook 账户
        outlook_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "outlook",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in outlook_services:
            config = service.config or {}
            result["outlook"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "outlook",
                "has_oauth": bool(config.get("client_id") and config.get("refresh_token")),
                "priority": service.priority
            })

        result["outlook"]["count"] = len(outlook_services)
        result["outlook"]["available"] = len(outlook_services) > 0

        # 获取自定义域名服务
        custom_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "moe_mail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in custom_services:
            config = service.config or {}
            result["moe_mail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "moe_mail",
                "default_domain": config.get("default_domain"),
                "priority": service.priority
            })

        result["moe_mail"]["count"] = len(custom_services)
        result["moe_mail"]["available"] = len(custom_services) > 0

        # 如果数据库中没有自定义域名服务，检查 settings
        if not result["moe_mail"]["available"]:
            if settings.custom_domain_base_url and settings.custom_domain_api_key:
                result["moe_mail"]["available"] = True
                result["moe_mail"]["count"] = 1
                result["moe_mail"]["services"].append({
                    "id": None,
                    "name": "默认自定义域名服务",
                    "type": "moe_mail",
                    "from_settings": True
                })

        # 获取 TempMail 服务（自部署 Cloudflare Worker 临时邮箱）
        temp_mail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "temp_mail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in temp_mail_services:
            config = service.config or {}
            result["temp_mail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "temp_mail",
                "domain": config.get("domain"),
                "priority": service.priority
            })

        result["temp_mail"]["count"] = len(temp_mail_services)
        result["temp_mail"]["available"] = len(temp_mail_services) > 0

        duck_mail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "duck_mail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in duck_mail_services:
            config = service.config or {}
            result["duck_mail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "duck_mail",
                "default_domain": config.get("default_domain"),
                "priority": service.priority
            })

        result["duck_mail"]["count"] = len(duck_mail_services)
        result["duck_mail"]["available"] = len(duck_mail_services) > 0

        freemail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "freemail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in freemail_services:
            config = service.config or {}
            result["freemail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "freemail",
                "domain": config.get("domain"),
                "priority": service.priority
            })

        result["freemail"]["count"] = len(freemail_services)
        result["freemail"]["available"] = len(freemail_services) > 0

        imap_mail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "imap_mail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in imap_mail_services:
            config = service.config or {}
            result["imap_mail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "imap_mail",
                "email": config.get("email"),
                "host": config.get("host"),
                "priority": service.priority
            })

        result["imap_mail"]["count"] = len(imap_mail_services)
        result["imap_mail"]["available"] = len(imap_mail_services) > 0

        luckmail_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "luckmail",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        for service in luckmail_services:
            config = service.config or {}
            result["luckmail"]["services"].append({
                "id": service.id,
                "name": service.name,
                "type": "luckmail",
                "project_code": config.get("project_code"),
                "email_type": config.get("email_type"),
                "preferred_domain": config.get("preferred_domain"),
                "priority": service.priority
            })

        result["luckmail"]["count"] = len(luckmail_services)
        result["luckmail"]["available"] = len(luckmail_services) > 0

    return result


# ============== Outlook 批量注册 API ==============

@router.get("/outlook-accounts", response_model=OutlookAccountsListResponse)
async def get_outlook_accounts_for_registration():
    """
    获取可用于注册的 Outlook 账户列表

    返回所有已启用的 Outlook 服务，并检查每个邮箱是否已在 accounts 表中注册
    """
    from ...database.models import EmailService as EmailServiceModel
    from ...database.models import Account

    with get_db() as db:
        # 获取所有启用的 Outlook 服务
        outlook_services = db.query(EmailServiceModel).filter(
            EmailServiceModel.service_type == "outlook",
            EmailServiceModel.enabled == True
        ).order_by(EmailServiceModel.priority.asc()).all()

        emails_by_service_id: Dict[int, str] = {}
        for service in outlook_services:
            config = service.config or {}
            email = config.get("email") or service.name
            emails_by_service_id[service.id] = email

        unique_emails = list(dict.fromkeys(email for email in emails_by_service_id.values() if email))
        existing_accounts_by_email = {}
        if unique_emails:
            existing_accounts_by_email = {
                row.email: row
                for row in db.query(
                    Account.id,
                    Account.email,
                    Account.refresh_token,
                ).filter(
                    Account.email.in_(unique_emails)
                ).all()
            }

        accounts = []
        registered_count = 0
        unregistered_count = 0

        for service in outlook_services:
            config = service.config or {}
            email = emails_by_service_id.get(service.id) or service.name
            existing_account = existing_accounts_by_email.get(email)

            is_registered = existing_account is not None
            has_refresh_token = _has_refresh_token(existing_account)
            needs_token_refresh = _needs_token_refresh(existing_account)
            is_registration_complete = _is_account_registration_complete(existing_account)
            if is_registered:
                registered_count += 1
            else:
                unregistered_count += 1

            accounts.append(OutlookAccountForRegistration(
                id=service.id,
                email=email,
                name=service.name,
                has_oauth=bool(config.get("client_id") and config.get("refresh_token")),
                is_registered=is_registered,
                has_refresh_token=has_refresh_token,
                needs_token_refresh=needs_token_refresh,
                is_registration_complete=is_registration_complete,
                registered_account_id=existing_account.id if existing_account else None
            ))

        return OutlookAccountsListResponse(
            total=len(accounts),
            registered_count=registered_count,
            unregistered_count=unregistered_count,
            accounts=accounts
        )


async def run_outlook_batch_registration(
    batch_id: str,
    service_ids: List[int],
    proxy: Optional[str],
    interval_min: int,
    interval_max: int,
    concurrency: int = 1,
    mode: str = "pipeline",
    auto_upload_cpa: bool = False,
    cpa_service_ids: List[int] = None,
    auto_upload_sub2api: bool = False,
    sub2api_service_ids: List[int] = None,
    auto_upload_tm: bool = False,
    tm_service_ids: List[int] = None,
    token_completion_concurrency: Optional[int] = None,
):
    """
    异步执行 Outlook 批量注册任务，复用通用并发逻辑

    将每个 service_id 映射为一个独立的 task_uuid，然后调用
    run_batch_registration 的并发逻辑
    """
    loop = task_manager.get_loop()
    if loop is None:
        loop = asyncio.get_event_loop()
        task_manager.set_loop(loop)

    # 预先为每个 service_id 创建注册任务记录
    task_uuids = []
    with get_db() as db:
        for service_id in service_ids:
            task_uuid = str(uuid.uuid4())
            crud.create_registration_task(
                db,
                task_uuid=task_uuid,
                proxy=proxy,
                email_service_id=service_id
            )
            task_uuids.append(task_uuid)

    task_manager.bind_batch_tasks(batch_id, task_uuids)
    if batch_tasks.get(batch_id, {}).get("cancelled"):
        task_manager.cancel_batch(batch_id)
    elif batch_tasks.get(batch_id, {}).get("paused"):
        task_manager.pause_batch(batch_id)

    # 复用通用并发逻辑（outlook 服务类型，每个任务通过 email_service_id 定位账户）
    await run_batch_registration(
        batch_id=batch_id,
        task_uuids=task_uuids,
        email_service_type="outlook",
        proxy=proxy,
        email_service_config=None,
        email_service_id=None,   # 每个任务已绑定了独立的 email_service_id
        interval_min=interval_min,
        interval_max=interval_max,
        concurrency=concurrency,
        mode=mode,
        auto_upload_cpa=auto_upload_cpa,
        cpa_service_ids=cpa_service_ids,
        auto_upload_sub2api=auto_upload_sub2api,
        sub2api_service_ids=sub2api_service_ids,
        auto_upload_tm=auto_upload_tm,
        tm_service_ids=tm_service_ids,
        token_completion_concurrency=token_completion_concurrency,
    )


@router.post("/outlook-batch", response_model=OutlookBatchRegistrationResponse)
async def start_outlook_batch_registration(
    request: OutlookBatchRegistrationRequest,
    background_tasks: BackgroundTasks
):
    """
    启动 Outlook 批量注册任务

    - service_ids: 选中的 EmailService ID 列表
    - proxy: 代理地址
    - interval_min: 最小间隔秒数
    - interval_max: 最大间隔秒数
    """
    # 验证参数
    if not request.service_ids:
        raise HTTPException(status_code=400, detail="请选择至少一个 Outlook 账户")

    if request.interval_min < 0 or request.interval_max < request.interval_min:
        raise HTTPException(status_code=400, detail="间隔时间参数无效")

    if not 1 <= request.concurrency <= 50:
        raise HTTPException(status_code=400, detail="并发数必须在 1-50 之间")

    if request.token_completion_concurrency is not None and not 1 <= request.token_completion_concurrency <= 50:
        raise HTTPException(status_code=400, detail="Token 收尾并发必须在 1-50 之间")

    if request.mode not in ("parallel", "pipeline"):
        raise HTTPException(status_code=400, detail="模式必须为 parallel 或 pipeline")

    actual_service_ids = list(request.service_ids)
    skipped_count = 0

    if not actual_service_ids:
        return OutlookBatchRegistrationResponse(
            batch_id="",
            total=len(request.service_ids),
            skipped=skipped_count,
            to_register=0,
            service_ids=[]
        )

    # 创建批量任务
    batch_id = str(uuid.uuid4())

    # 初始化批量任务状态
    batch_tasks[batch_id] = {
        "status": "running",
        "total": len(actual_service_ids),
        "completed": 0,
        "success": 0,
        "failed": 0,
        "skipped": 0,
        "cancelled": False,
        "paused": False,
        "service_ids": actual_service_ids,
        "current_index": 0,
        "logs": [],
        "finished": False
    }

    # 在后台运行批量注册
    background_tasks.add_task(
        run_outlook_batch_registration,
        batch_id,
        actual_service_ids,
        request.proxy,
        request.interval_min,
        request.interval_max,
        concurrency=request.concurrency,
        mode=request.mode,
        auto_upload_cpa=request.auto_upload_cpa,
        cpa_service_ids=request.cpa_service_ids,
        auto_upload_sub2api=request.auto_upload_sub2api,
        sub2api_service_ids=request.sub2api_service_ids,
        auto_upload_tm=request.auto_upload_tm,
        tm_service_ids=request.tm_service_ids,
        token_completion_concurrency=request.token_completion_concurrency,
    )

    return OutlookBatchRegistrationResponse(
        batch_id=batch_id,
        total=len(request.service_ids),
        skipped=skipped_count,
        to_register=len(actual_service_ids),
        service_ids=actual_service_ids
    )


@router.get("/outlook-batch/{batch_id}")
async def get_outlook_batch_status(batch_id: str, log_offset: int = Query(0, ge=0)):
    """获取 Outlook 批量任务状态"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    runtime_logs = task_manager.get_batch_logs(batch_id)
    log_base_index = task_manager.get_batch_log_base_index(batch_id) if runtime_logs else 0
    resolved_logs = runtime_logs if runtime_logs else batch.get("logs", [])
    incremental_logs, normalized_offset, log_next_offset = _resolve_log_window(
        resolved_logs,
        offset=log_offset,
        base_index=log_base_index,
    )
    return {
        "batch_id": batch_id,
        "status": batch.get("status", "running"),
        "total": batch["total"],
        "completed": batch["completed"],
        "success": batch["success"],
        "failed": batch["failed"],
        "skipped": batch.get("skipped", 0),
        "current_index": batch["current_index"],
        "cancelled": batch["cancelled"],
        "paused": batch.get("paused", False),
        "finished": batch.get("finished", False),
        "logs": incremental_logs,
        "log_base_index": log_base_index,
        "log_offset": normalized_offset,
        "log_next_offset": log_next_offset,
        "progress": f"{batch['completed']}/{batch['total']}"
    }


@router.post("/outlook-batch/{batch_id}/pause")
async def pause_outlook_batch(batch_id: str):
    """暂停 Outlook 批量任务。"""
    return await pause_batch(batch_id)


@router.post("/outlook-batch/{batch_id}/resume")
async def resume_outlook_batch(batch_id: str):
    """恢复 Outlook 批量任务。"""
    return await resume_batch(batch_id)


@router.post("/outlook-batch/{batch_id}/cancel")
async def cancel_outlook_batch(batch_id: str):
    """取消 Outlook 批量任务"""
    if batch_id not in batch_tasks:
        raise HTTPException(status_code=404, detail="批量任务不存在")

    batch = batch_tasks[batch_id]
    if batch.get("finished"):
        raise HTTPException(status_code=400, detail="批量任务已完成")

    # 同时更新两个系统的取消状态
    batch["cancelled"] = True
    batch["paused"] = False
    batch["status"] = "cancelling"
    task_manager.bind_batch_tasks(batch_id, batch.get("task_uuids", []))
    task_manager.cancel_batch(batch_id)

    return {"success": True, "message": "批量任务取消请求已提交，正在让它们有序收工"}
