package registration

const pythonRunnerScript = `
import json
import os
import sys
import traceback
from datetime import datetime, timezone


def emit(payload):
    sys.stdout.write(json.dumps(payload, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def log(message, level="info"):
    emit({"type": "log", "level": str(level or "info"), "message": str(message or "")})


def read_control_state(control_path):
    control_path = str(control_path or "").strip()
    if not control_path:
        return "running"

    try:
        with open(control_path, "r", encoding="utf-8") as handle:
            state = str(handle.read() or "").strip().lower()
    except FileNotFoundError:
        return "running"
    except Exception:
        return "running"

    if state in ("paused", "cancelled", "running"):
        return state
    return "running"


def build_check_cancelled(control_path):
    control_path = str(control_path or "").strip()
    if not control_path:
        return None

    def callback():
        while True:
            state = read_control_state(control_path)
            if state == "cancelled":
                return True
            if state != "paused":
                return False
            time.sleep(0.1)

    return callback


def format_optional_datetime(value):
    if value in (None, ""):
        return None
    if isinstance(value, str):
        stripped = value.strip()
        return stripped or None
    if isinstance(value, datetime):
        if value.tzinfo is None:
            return value.replace(microsecond=0).isoformat() + "Z"
        return value.astimezone(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

    isoformat = getattr(value, "isoformat", None)
    if callable(isoformat):
        try:
            formatted = str(isoformat() or "").strip()
        except Exception:
            return None
        return formatted or None
    return None


def resolve_optional_account_field(result, metadata, key, formatter=None):
    value = getattr(result, key, None)
    if value in (None, ""):
        value = (metadata or {}).get(key)
    if value in (None, ""):
        return None

    if formatter is None:
        formatted = str(value or "").strip()
        return formatted or None
    return formatter(value)


def normalize_email_service_config(service_type, config, proxy_url=None):
    normalized = dict(config or {})

    if "api_url" in normalized and "base_url" not in normalized:
        normalized["base_url"] = normalized.pop("api_url")

    if service_type == "moe_mail":
        if "domain" in normalized and "default_domain" not in normalized:
            normalized["default_domain"] = normalized.pop("domain")
    elif service_type in ("tempmail", "freemail"):
        if "default_domain" in normalized and "domain" not in normalized:
            normalized["domain"] = normalized.pop("default_domain")
    elif service_type == "duck_mail":
        if "domain" in normalized and "default_domain" not in normalized:
            normalized["default_domain"] = normalized.pop("domain")
    elif service_type == "luckmail":
        if "domain" in normalized and "preferred_domain" not in normalized:
            normalized["preferred_domain"] = normalized.pop("domain")

    if proxy_url and "proxy_url" not in normalized:
        normalized["proxy_url"] = proxy_url

    return normalized


def resolve_registration_config(payload):
    from src.config.settings import get_settings
    from src.database.session import get_db
    from src.database.models import EmailService as EmailServiceModel

    settings = get_settings()
    service_type = str(payload.get("email_service_type") or "").strip() or "tempmail"
    requested_proxy = str(payload.get("proxy") or "").strip() or None
    requested_config = payload.get("email_service_config") or {}
    requested_service_id = payload.get("email_service_id")

    if requested_service_id not in (None, ""):
        requested_service_id = int(requested_service_id)
        with get_db() as db:
            service = (
                db.query(EmailServiceModel)
                .filter(
                    EmailServiceModel.id == requested_service_id,
                    EmailServiceModel.enabled == True,
                )
                .first()
            )
            if service is None:
                raise ValueError(f"邮箱服务不存在或已禁用: {requested_service_id}")

            resolved_type = str(service.service_type or "").strip() or service_type
            resolved_config = normalize_email_service_config(
                resolved_type,
                service.config or {},
                requested_proxy,
            )
            log(f"使用数据库邮箱服务: {service.name} (ID: {service.id}, 类型: {resolved_type})")
            return resolved_type, resolved_config, int(service.id)

    if requested_config:
        return service_type, normalize_email_service_config(service_type, requested_config, requested_proxy), None

    if service_type == "tempmail":
        if not settings.tempmail_enabled:
            raise ValueError("Tempmail.lol 渠道已禁用，请先在邮箱服务页面启用")
        return service_type, {
            "base_url": settings.tempmail_base_url,
            "timeout": settings.tempmail_timeout,
            "max_retries": settings.tempmail_max_retries,
            "proxy_url": requested_proxy,
        }, None

    if service_type == "moe_mail":
        from src.database.session import get_db
        from src.database.models import EmailService as EmailServiceModel

        with get_db() as db:
            service = (
                db.query(EmailServiceModel)
                .filter(
                    EmailServiceModel.service_type == "moe_mail",
                    EmailServiceModel.enabled == True,
                )
                .order_by(EmailServiceModel.priority.asc())
                .first()
            )
            if service is not None and service.config:
                resolved_config = normalize_email_service_config(service_type, service.config, requested_proxy)
                log(f"使用数据库自定义域名服务: {service.name}")
                return service_type, resolved_config, int(service.id)

        if settings.custom_domain_base_url and settings.custom_domain_api_key:
            return service_type, {
                "base_url": settings.custom_domain_base_url,
                "api_key": settings.custom_domain_api_key.get_secret_value() if settings.custom_domain_api_key else "",
                "proxy_url": requested_proxy,
            }, None

        raise ValueError("没有可用的自定义域名邮箱服务，请先在设置中配置")

    if service_type == "outlook":
        raise ValueError("Outlook registration requires email_service_id or email_service_config")

    return service_type, normalize_email_service_config(service_type, requested_config, requested_proxy), None


def run_optional_uploads(payload, result, log_callback):
    from src.database import crud
    from src.database.models import Account as AccountModel
    from src.database.session import get_db

    email = str(getattr(result, "email", "") or "").strip()
    if not email:
        return

    with get_db() as db:
        saved_account = db.query(AccountModel).filter_by(email=email).first()
        if saved_account is None or not getattr(saved_account, "access_token", None):
            if payload.get("auto_upload_cpa"):
                log_callback("[CPA] 账号未落库或缺少 access_token，跳过上传")
            if payload.get("auto_upload_sub2api"):
                log_callback("[Sub2API] 账号未落库或缺少 access_token，跳过上传")
            if payload.get("auto_upload_tm"):
                log_callback("[TM] 账号未落库或缺少 access_token，跳过上传")
            return

        if payload.get("auto_upload_cpa"):
            try:
                from src.core.upload.cpa_upload import generate_token_json, upload_to_cpa

                service_ids = list(payload.get("cpa_service_ids") or [])
                if not service_ids:
                    service_ids = [service.id for service in crud.get_cpa_services(db, enabled=True)]
                if not service_ids:
                    log_callback("[CPA] 无可用 CPA 服务，跳过上传")
                else:
                    token_data = generate_token_json(saved_account)
                    for service_id in service_ids:
                        service = crud.get_cpa_service_by_id(db, service_id)
                        if service is None:
                            continue
                        log_callback(f"[CPA] 正在把账号打包发往服务站: {service.name}")
                        ok, message = upload_to_cpa(token_data, api_url=service.api_url, api_token=service.api_token)
                        if ok:
                            saved_account.cpa_uploaded = True
                            saved_account.cpa_uploaded_at = datetime.utcnow()
                            db.commit()
                            log_callback(f"[CPA] 投递成功，服务站已签收: {service.name}")
                        else:
                            log_callback(f"[CPA] 上传失败({service.name}): {message}")
            except Exception as exc:
                log_callback(f"[CPA] 上传异常: {exc}")

        if payload.get("auto_upload_sub2api"):
            try:
                from src.core.upload.sub2api_upload import upload_to_sub2api

                service_ids = list(payload.get("sub2api_service_ids") or [])
                if not service_ids:
                    service_ids = [service.id for service in crud.get_sub2api_services(db, enabled=True)]
                if not service_ids:
                    log_callback("[Sub2API] 无可用 Sub2API 服务，跳过上传")
                else:
                    for service_id in service_ids:
                        service = crud.get_sub2api_service_by_id(db, service_id)
                        if service is None:
                            continue
                        log_callback(f"[Sub2API] 正在把账号发往服务站: {service.name}")
                        ok, message = upload_to_sub2api([saved_account], service.api_url, service.api_key)
                        if ok:
                            saved_account.sub2api_uploaded = True
                            saved_account.sub2api_uploaded_at = datetime.utcnow()
                            db.commit()
                        log_callback(f"[Sub2API] {'成功' if ok else '失败'}({service.name}): {message}")
            except Exception as exc:
                log_callback(f"[Sub2API] 上传异常: {exc}")

        if payload.get("auto_upload_tm"):
            try:
                from src.core.upload.team_manager_upload import upload_to_team_manager

                service_ids = list(payload.get("tm_service_ids") or [])
                if not service_ids:
                    service_ids = [service.id for service in crud.get_tm_services(db, enabled=True)]
                if not service_ids:
                    log_callback("[TM] 无可用 Team Manager 服务，跳过上传")
                else:
                    for service_id in service_ids:
                        service = crud.get_tm_service_by_id(db, service_id)
                        if service is None:
                            continue
                        log_callback(f"[TM] 正在把账号发往服务站: {service.name}")
                        ok, message = upload_to_team_manager(saved_account, service.api_url, service.api_key)
                        log_callback(f"[TM] {'成功' if ok else '失败'}({service.name}): {message}")
            except Exception as exc:
                log_callback(f"[TM] 上传异常: {exc}")


def build_account_persistence_payload(engine, result, service_type, resolved_service_id, proxy_url):
    from src.config.settings import get_settings

    settings = get_settings()
    metadata = dict(getattr(result, "metadata", None) or {})
    if resolved_service_id is not None:
        metadata["email_service_id"] = int(resolved_service_id)
        email_service_id = str(resolved_service_id)
    else:
        email_info = getattr(engine, "email_info", None) or {}
        email_service_id = str(email_info.get("service_id") or "").strip()
    metadata["go_executor"] = "python_bridge"

    device_id = str(getattr(result, "device_id", "") or "").strip()
    if device_id:
        metadata["device_id"] = device_id

    status = engine._resolve_persisted_account_status(result, metadata)

    payload = {
        "email": str(getattr(result, "email", "") or "").strip(),
        "password": str(getattr(result, "password", "") or ""),
        "client_id": str(getattr(settings, "openai_client_id", "") or "").strip(),
        "session_token": str(getattr(result, "session_token", "") or "").strip(),
        "email_service": str(service_type or "").strip(),
        "email_service_id": email_service_id,
        "account_id": str(getattr(result, "account_id", "") or "").strip(),
        "workspace_id": str(getattr(result, "workspace_id", "") or "").strip(),
        "access_token": str(getattr(result, "access_token", "") or "").strip(),
        "refresh_token": str(getattr(result, "refresh_token", "") or "").strip(),
        "id_token": str(getattr(result, "id_token", "") or "").strip(),
        "cookies": str(engine._dump_session_cookies() or ""),
        "proxy_used": str(proxy_url or "").strip(),
        "extra_data": metadata,
        "status": str(status or "").strip(),
        "source": str(getattr(result, "source", "") or "").strip() or "register",
    }
    last_refresh = resolve_optional_account_field(result, metadata, "last_refresh", format_optional_datetime)
    if last_refresh:
        payload["last_refresh"] = last_refresh

    expires_at = resolve_optional_account_field(result, metadata, "expires_at", format_optional_datetime)
    if expires_at:
        payload["expires_at"] = expires_at

    subscription_type = resolve_optional_account_field(result, metadata, "subscription_type")
    if subscription_type:
        payload["subscription_type"] = subscription_type

    subscription_at = resolve_optional_account_field(result, metadata, "subscription_at", format_optional_datetime)
    if subscription_at:
        payload["subscription_at"] = subscription_at
    return payload, metadata


def main():
    repo_root = os.getcwd()
    if repo_root not in sys.path:
        sys.path.insert(0, repo_root)

    control_path = os.getenv("CODEX_CONSOLE_RUNNER_CONTROL_PATH", "")
    payload = json.load(sys.stdin)
    go_persistence_enabled = bool(payload.get("go_persistence_enabled"))
    start_request = payload.get("start_request") or {}
    plan = payload.get("plan") or {}

    from src.core.register import RegistrationEngine
    from src.services import EmailServiceFactory, EmailServiceType

    prepared_email = plan.get("email_service") or {}
    prepared_proxy = plan.get("proxy") or {}
    proxy_url = str(prepared_proxy.get("selected") or start_request.get("proxy") or "").strip() or None

    if prepared_email.get("prepared"):
        service_type = str(prepared_email.get("type") or start_request.get("email_service_type") or "").strip() or "tempmail"
        service_config = dict(prepared_email.get("config") or {})
        resolved_service_id = prepared_email.get("service_id")
    else:
        service_type, service_config, resolved_service_id = resolve_registration_config(start_request)

    email_service = EmailServiceFactory.create(EmailServiceType(service_type), service_config)

    def log_callback(message):
        log(message, "info")

    engine = RegistrationEngine(
        email_service=email_service,
        proxy_url=proxy_url,
        callback_logger=log_callback,
        task_uuid=str(payload.get("task_uuid") or plan.get("task", {}).get("task_uuid") or "").strip() or None,
        check_cancelled=build_check_cancelled(control_path),
    )

    result = engine.run()
    result_dict = result.to_dict()
    account_persistence, metadata = build_account_persistence_payload(
        engine,
        result,
        service_type,
        resolved_service_id,
        proxy_url,
    )
    result.metadata = metadata
    result_dict["metadata"] = metadata

    if result.success and not go_persistence_enabled:
        saved = False
        try:
            saved = bool(engine.save_to_database(result))
        except Exception as exc:
            log(f"保存到数据库失败: {exc}", "warning")
        if not saved:
            log("账号落库未确认成功，兼容 Python 语义继续返回 completed", "warning")

        run_optional_uploads(start_request, result, log_callback)

    emit({
        "type": "result",
        "success": bool(result.success),
        "result": result_dict,
        "account_persistence": account_persistence if result.success else None,
        "error_message": str(getattr(result, "error_message", "") or ""),
    })


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        emit({
            "type": "fatal",
            "success": False,
            "error_message": str(exc),
        })
        traceback.print_exc(file=sys.stderr)
        raise
`
