-- Migration 0009: Seed Auth Transactional Email Templates
-- Seeds initial templates for GMHelper auth flows in all 8 supported locales:
-- 1. auth.register_code
-- 2. auth.welcome
-- 3. auth.password_recovery
-- 4. auth.ip_confirmation

-- 1. auth.register_code
INSERT INTO email_templates (id, template_key, name, template_type, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES
(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', 'Email confirmation for registration',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hello,</p>
    <p>To complete your registration, please enter the verification code below:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>This code will expire in 5 minutes. If you did not request registration, you can ignore this email.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'en', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', 'Код подтверждения регистрации',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Здравствуйте,</p>
    <p>Для завершения регистрации введите код подтверждения ниже:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Код действует 5 минут. Если вы не регистрировались — проигнорируйте это письмо.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Все права защищены.</footer>
</div>', '', 'ru', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', 'Код підтвердження реєстрації',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Вітаємо,</p>
    <p>Щоб завершити реєстрацію, введіть код підтвердження нижче:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Код дійсний 5 хвилин. Якщо це були не ви — проігноруйте лист.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Усі права захищені.</footer>
</div>', '', 'uk', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', 'Code de confirmation d’inscription',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Bonjour,</p>
    <p>Pour finaliser votre inscription, veuillez entrer le code ci-dessous :</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Ce code est valable 5 minutes. Si vous n’êtes pas à l’origine de cette demande, ignorez cet email.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Tous droits réservés.</footer>
</div>', '', 'fr', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', 'Registrierungsbestätigungscode',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hallo,</p>
    <p>Um die Registrierung abzuschließen, geben Sie bitte den folgenden Code ein:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Dieser Code ist 5 Minuten gültig. Falls Sie sich nicht registriert haben, ignorieren Sie diese E-Mail.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Alle Rechte vorbehalten.</footer>
</div>', '', 'de', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', '登録確認コード',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>こんにちは、</p>
    <p>登録を完了するために、以下のコードを入力してください：</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>このコードは5分間有効です。心当たりがない場合は無視してください。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ja', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', '회원가입 인증 코드',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>안녕하세요,</p>
    <p>회원가입을 완료하려면 아래 인증 코드를 입력하세요:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>이 코드는 5분 동안 유효합니다. 본인이 아니라면 이 이메일을 무시하세요.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ko', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.register_code', 'Registration Verification Code', 'direct', '注册验证码',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>你好，</p>
    <p>请输入以下验证码以完成注册：</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>验证码5分钟内有效。如果不是您操作，请忽略此邮件。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. 版权所有.</footer>
</div>', '', 'zh', 'active', 1, now(), now())
ON CONFLICT (template_key, locale, version) DO NOTHING;

-- 2. auth.password_recovery
INSERT INTO email_templates (id, template_key, name, template_type, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES
(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'Password Recovery',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hello!</p>
    <p>We received a request to reset your password.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Reset Password</a>
    </p>
    <p>This link will expire in 15 minutes.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'en', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'Восстановление пароля',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Здравствуйте!</p>
    <p>Мы получили запрос на сброс вашего пароля.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Сбросить пароль</a>
    </p>
    <p>Ссылка действительна в течение 15 минут.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Все права защищены.</footer>
</div>', '', 'ru', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'Відновлення пароля',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Вітаємо!</p>
    <p>Ми отримали запит на скидання вашого пароля.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Скинути пароль</a>
    </p>
    <p>Посилання дійсне протягом 15 хвилин.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Усі права захищені.</footer>
</div>', '', 'uk', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'Récupération de mot de passe',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Bonjour&nbsp;!</p>
    <p>Nous avons reçu une demande de réinitialisation de votre mot de passe.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Réinitialiser le mot de passe</a>
    </p>
    <p>Ce lien expirera dans 15 minutes.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Tous droits réservés.</footer>
</div>', '', 'fr', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'Passwort wiederherstellen',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hallo!</p>
    <p>Wir haben eine Anfrage zum Zurücksetzen Ihres Passworts erhalten.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Passwort zurücksetzen</a>
    </p>
    <p>Dieser Link ist 15 Minuten gültig.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Alle Rechte vorbehalten.</footer>
</div>', '', 'de', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', 'パスワードの再設定',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>こんにちは！</p>
    <p>パスワード再設定のリクエストを受信しました。</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>パスワードをリセット</a>
    </p>
    <p>このリンクは15分間有効です。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ja', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', '비밀번호 재설정',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>안녕하세요!</p>
    <p>비밀번호 재설정 요청을 받았습니다.</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>비밀번호 재설정</a>
    </p>
    <p>이 링크는 15분 동안 유효합니다.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ko', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.password_recovery', 'Password Recovery', 'direct', '重置密码',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>你好！</p>
    <p>我们收到了重置您密码的请求。</p>
    <p style=''text-align:center;''>
        <a href=''{{ recoveryLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>重置密码</a>
    </p>
    <p>该链接将在15分钟后失效。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. 版权所有.</footer>
</div>', '', 'zh', 'active', 1, now(), now())
ON CONFLICT (template_key, locale, version) DO NOTHING;

-- 3. auth.ip_confirmation
INSERT INTO email_templates (id, template_key, name, template_type, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES
(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', 'New device login confirmation',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hello,</p>
    <p>We detected a login attempt from a new device or IP address. To confirm this login, please enter the code below:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>This code will expire in 15 minutes. If you did not attempt to log in, please ignore this email.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'en', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', 'Подтверждение входа с нового устройства',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Здравствуйте,</p>
    <p>Мы обнаружили попытку входа с нового устройства или IP-адреса. Для подтверждения входа введите код ниже:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Код действителен в течение 15 минут. Если это были не вы — просто проигнорируйте письмо.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Все права защищены.</footer>
</div>', '', 'ru', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', 'Підтвердження входу з нового пристрою',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Вітаємо,</p>
    <p>Ми виявили спробу входу з нового пристрою або IP-адреси. Для підтвердження входу введіть код нижче:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Код дійсний протягом 15 хвилин. Якщо це були не ви — проігноруйте цей лист.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Усі права захищені.</footer>
</div>', '', 'uk', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', 'Confirmation de connexion depuis un nouvel appareil',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Bonjour,</p>
    <p>Nous avons détecté une tentative de connexion depuis un nouvel appareil ou une nouvelle adresse IP. Pour confirmer la connexion, veuillez saisir le code ci-dessous :</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Ce code expirera dans 15 minutes. Si vous n’êtes pas à l’origine de cette tentative, ignorez cet e-mail.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Tous droits réservés.</footer>
</div>', '', 'fr', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', 'Anmeldung von neuem Gerät bestätigen',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>Hallo,</p>
    <p>Wir haben einen Anmeldeversuch von einem neuen Gerät oder einer neuen IP-Adresse festgestellt. Bitte geben Sie den folgenden Code ein, um die Anmeldung zu bestätigen:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>Der Code ist 15 Minuten gültig. Falls Sie dies nicht waren, ignorieren Sie bitte diese E-Mail.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. Alle Rechte vorbehalten.</footer>
</div>', '', 'de', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', '新しいデバイスからのログイン確認',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>こんにちは、</p>
    <p>新しいデバイスまたはIPアドレスからのログイン試行が検出されました。確認のため、以下のコードを入力してください：</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>このコードは15分間有効です。心当たりがない場合は、このメールを無視してください。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ja', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', '새 기기 로그인 확인',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>안녕하세요,</p>
    <p>새로운 기기 또는 IP 주소에서 로그인 시도가 감지되었습니다. 로그인을 확인하려면 아래 코드를 입력하세요:</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>이 코드는 15분 동안 유효합니다. 본인이 아니라면 이 이메일을 무시하세요.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. All rights reserved.</footer>
</div>', '', 'ko', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.ip_confirmation', 'New Device Login Confirmation', 'direct', '新设备登录确认',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p>你好，</p>
    <p>我们检测到来自新设备或新 IP 地址的登录尝试。请输入以下验证码以确认登录：</p>
    <p style=''text-align:center; font-size:24px; font-weight:bold; color:#C444FF;''>{{ code }}</p>
    <p>该验证码将在 15 分钟后失效。如果这不是您的操作，请忽略此邮件。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>&copy; GMHelper. 版权所有.</footer>
</div>', '', 'zh', 'active', 1, now(), now())
ON CONFLICT (template_key, locale, version) DO NOTHING;

-- 4. auth.welcome
INSERT INTO email_templates (id, template_key, name, template_type, subject, html_body, plain_text_body, locale, status, version, created_at, updated_at)
VALUES
(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'Welcome to GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>Hello,</p>
    <p style=''color:#fff;''>Welcome to GMHelper!</p>
    <p style=''color:#fff;''>Your registration has been completed successfully and your email address has been verified.</p>
    <p style=''color:#fff;''>You can now sign in and start using all available features of the platform.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Open GMHelper</a>
    </p>
    <p style=''color:#fff;''>If you did not create this account, please contact support as soon as possible.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. All rights reserved.
    </footer>
</div>', '', 'en', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'Добро пожаловать в GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>Здравствуйте,</p>
    <p style=''color:#fff;''>Добро пожаловать в GMHelper!</p>
    <p style=''color:#fff;''>Ваша регистрация успешно завершена, а адрес электронной почты подтвержден.</p>
    <p style=''color:#fff;''>Теперь вы можете войти в систему и пользоваться всеми возможностями платформы.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Открыть GMHelper</a>
    </p>
    <p style=''color:#fff;''>Если вы не создавали этот аккаунт, пожалуйста, обратитесь в службу поддержки.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. Все права защищены.
    </footer>
</div>', '', 'ru', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'Ласкаво просимо до GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>Вітаємо,</p>
    <p style=''color:#fff;''>Ласкаво просимо до GMHelper!</p>
    <p style=''color:#fff;''>Вашу реєстрацію успішно завершено, а адресу електронної пошти підтверджено.</p>
    <p style=''color:#fff;''>Тепер ви можете увійти та користуватися всіма можливостями платформи.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Відкрити GMHelper</a>
    </p>
    <p style=''color:#fff;''>Якщо ви не створювали цей обліковий запис, зверніться до служби підтримки.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. Усі права захищені.
    </footer>
</div>', '', 'uk', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'Bienvenue sur GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>Bonjour,</p>
    <p style=''color:#fff;''>Bienvenue sur GMHelper&nbsp;!</p>
    <p style=''color:#fff;''>Votre inscription a été finalisée avec succès et votre adresse e-mail a été vérifiée.</p>
    <p style=''color:#fff;''>Vous pouvez maintenant vous connecter et profiter de toutes les fonctionnalités.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>Ouvrir GMHelper</a>
    </p>
    <p style=''color:#fff;''>Si vous n’êtes pas à l’origine de ce compte, veuillez contacter le support dès que possible.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. Tous droits réservés.
    </footer>
</div>', '', 'fr', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'Willkommen bei GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>Hallo,</p>
    <p style=''color:#fff;''>Willkommen bei GMHelper!</p>
    <p style=''color:#fff;''>Ihre Registrierung wurde erfolgreich abgeschlossen und Ihre E-Mail-Adresse bestätigt.</p>
    <p style=''color:#fff;''>Sie können sich jetzt anmelden und alle Funktionen der Plattform nutzen.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>GMHelper öffnen</a>
    </p>
    <p style=''color:#fff;''>Falls Sie dieses Konto nicht erstellt haben, wenden Sie sich bitte an den Support.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. Alle Rechte vorbehalten.
    </footer>
</div>', '', 'de', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'GMHelperへようこそ',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>こんにちは、</p>
    <p style=''color:#fff;''>GMHelperへようこそ！</p>
    <p style=''color:#fff;''>登録が正常に完了し、メールアドレスが認証されました。</p>
    <p style=''color:#fff;''>ログインして、すべての機能をご利用いただけます。</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>GMHelperを開く</a>
    </p>
    <p style=''color:#fff;''>このアカウントを作成した覚えがない場合は、サポートまでお問い合わせください。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. All rights reserved.
    </footer>
</div>', '', 'ja', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', 'GMHelper에 오신 것을 환영합니다',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>안녕하세요,</p>
    <p style=''color:#fff;''>GMHelper에 오신 것을 환영합니다!</p>
    <p style=''color:#fff;''>회원가입이 성공적으로 완료되었으며 이메일 인증이 완료되었습니다.</p>
    <p style=''color:#fff;''>이제 로그인하여 플랫폼의 모든 기능을 이용하실 수 있습니다.</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>GMHelper 열기</a>
    </p>
    <p style=''color:#fff;''>본인이 계정을 생성하지 않았다면 지원팀으로 문의해 주시기 바랍니다.</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. All rights reserved.
    </footer>
</div>', '', 'ko', 'active', 1, now(), now()),

(uuid_generate_v4(), 'auth.welcome', 'Welcome to GMHelper', 'direct', '欢迎来到 GMHelper',
'<div style=''background-color:#000; color:#fff; font-family:Arial, sans-serif; max-width:600px; margin:auto; padding:20px; border-radius:10px;''>
    <h2 style=''text-align:center;''>
        <a href=''{{ mainLink }}'' style=''text-decoration:none; font-size:28px;''>
            <span style=''color:#C444FF;''>GM</span><span style=''color:#FFFFFF;''>Helper</span>
        </a>
    </h2>
    <p style=''color:#fff;''>你好，</p>
    <p style=''color:#fff;''>欢迎来到 GMHelper！</p>
    <p style=''color:#fff;''>您的注册已成功完成，电子邮箱已验证。</p>
    <p style=''color:#fff;''>您现在可以登录并开始使用平台的全部功能。</p>
    <p style=''text-align:center; margin:30px 0;''>
        <a href=''{{ mainLink }}'' style=''display:inline-block; padding:12px 24px; background-color:#C444FF; color:#fff; text-decoration:none; border-radius:5px;''>打开 GMHelper</a>
    </p>
    <p style=''color:#fff;''>如果这不是您创建的账户，请尽快联系支持团队。</p>
    <hr style=''border-color:#444;''/>
    <footer style=''text-align:center; font-size:12px; color:#666;''>
        &copy; GMHelper. 版权所有.
    </footer>
</div>', '', 'zh', 'active', 1, now(), now())
ON CONFLICT (template_key, locale, version) DO NOTHING;
