// Carga las variables de entorno desde el archivo .env
require('dotenv').config();
const express = require('express');
const { google } = require('googleapis');
const path = require('path');
const nodemailer = require('nodemailer');
// const bcrypt = require('bcryptjs'); // Descomentar si decides usar hashing de contraseñas

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware para JSON y CORS
app.use(express.json());
app.use((req, res, next) => {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.header('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  next();
});

// Sirve archivos estáticos (HTML, CSS, JS, imágenes) desde la carpeta 'public'
app.use(express.static(path.join(__dirname, 'public')));

// Configuración de Google Sheets
const SPREADSHEET_ID = '1I6pVLSBav-U7c86FLavh0tikPghLDVrWCFuru-qwQ4Y'; // Tu ID de Spreadsheet

const serviceAccount = JSON.parse(process.env.GOOGLE_SERVICE_ACCOUNT);
const auth = new google.auth.GoogleAuth({
  credentials: serviceAccount,
  scopes: ['https://www.googleapis.com/auth/spreadsheets']
});

let sheets;
async function initializeGoogleSheetsClient() {
  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });
    console.log('Google Sheets client initialized.');
  } catch (error) {
    console.error('Error initializing Google Sheets client:', error);
    process.exit(1); // Salir si no se puede inicializar
  }
}

// Función auxiliar para obtener el nombre de la hoja según la carrera
function getSheetName(career) {
    if (career === 'Diseno Digital') {
        return 'Sheets1';
    } else if (career === 'Diseno de la Comunicacion') {
        return 'Com';
    } else {
        throw new Error('Carrera no válida');
    }
}

// Función para calcular el nivel basada en la EXP
function calculateLevel(exp) {
    if (exp >= 100) return 5;
    if (exp >= 75) return 4;
    if (exp >= 50) return 3;
    if (exp >= 25) return 2;
    if (exp >= 10) return 1;
    return 0;
}

// Inicializar el cliente de Google Sheets al inicio de la aplicación
initializeGoogleSheetsClient();

// ===============================================
// === ENDPOINTS DE LA API ===
// ===============================================

// Endpoint para el login
app.post('/api/login', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { studentId, password, career } = req.body;

    if (!studentId || !password || !career) {
        return res.status(400).json({ error: 'ID de alumno, contraseña y carrera son requeridos.' });
    }

    const sheetName = getSheetName(career);

    try {
        const response = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!A:L`, // Obtener todas las columnas relevantes
        });
        const rows = response.data.values;

        if (!rows || rows.length === 0) {
            return res.status(404).json({ error: 'No se encontraron alumnos en esta carrera.' });
        }

        let studentFound = null;
        let studentRowIndex = -1;
        let levelUpOccurred = false;

        for (let i = 1; i < rows.length; i++) { // Empezar desde la fila 1 (índice 1) para omitir el encabezado
            const row = rows[i];
            const currentId = row[0]; // Columna A
            const currentPassword = row[8]; // Columna I
            
            // Reemplaza esto con bcrypt.compare si usas hashing
            // const passwordMatch = await bcrypt.compare(password, currentPassword);
            const passwordMatch = (password === currentPassword); // Comparación simple para tu formato actual

            if (currentId === studentId && passwordMatch) {
                const name = row[1]; // Columna B
                const sexo = row[2]; // Columna C
                const monedas = parseInt(row[3]) || 0; // Columna D
                const tareas = parseInt(row[4]) || 0; // Columna E
                const asistencias = parseInt(row[5]) || 0; // Columna F
                const comprasString = row[6] || '{}'; // Columna G
                const exp = parseInt(row[9]) || 0; // Columna J
                let lastLoginLevel = parseInt(row[10]) || 0; // Columna K (LAST_LOGIN_LEVEL)
                const studentLevel = calculateLevel(exp);

                let purchases = {};
                try {
                    purchases = JSON.parse(comprasString);
                } catch (e) {
                    console.error('Error al parsear compras:', e);
                }

                // Verificar si hay un "level up" desde el último login
                if (studentLevel > lastLoginLevel) {
                    levelUpOccurred = true;
                    // Actualizar LAST_LOGIN_LEVEL en la hoja de cálculo
                    const rowNumber = i + 1; // Fila en Sheets (base 1)
                    await sheets.spreadsheets.values.update({
                        spreadsheetId: SPREADSHEET_ID,
                        range: `${sheetName}!K${rowNumber}`, // Columna K
                        valueInputOption: 'RAW',
                        requestBody: {
                            values: [[studentLevel]],
                        },
                    });
                    console.log(`Nivel de ${studentId} actualizado a ${studentLevel} en LAST_LOGIN_LEVEL.`);
                }

                studentFound = {
                    id: studentId,
                    name,
                    sexo,
                    monedas,
                    tareas,
                    asistencias,
                    purchases,
                    exp,
                    level: studentLevel,
                    rowIndex: i, // Guardar el índice de la fila para futuras actualizaciones
                };
                studentRowIndex = i; // Guardar el índice de la fila para update
                break;
            }
        }

        if (studentFound) {
            res.json({ success: true, student: studentFound, levelUpOccurred });
        } else {
            res.status(401).json({ success: false, error: 'ID o contraseña incorrectos.' });
        }

    } catch (error) {
        console.error('Error al iniciar sesión:', error);
        res.status(500).json({ error: 'Error interno del servidor.' });
    }
});

// Endpoint para registrar nuevo usuario
app.post('/api/register', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { id, name, sexo, password, career } = req.body;

    if (!id || !name || !sexo || !password || !career) {
        return res.status(400).json({ error: 'Todos los campos son requeridos.' });
    }

    const sheetName = getSheetName(career);

    // Validar formato de contraseña (4 caracteres HEX mayúsculas)
    const passwordRegex = /^[A-F0-9]{4}$/;
    if (!passwordRegex.test(password)) {
      return res.status(400).json({ error: 'La contraseña debe ser de 4 caracteres (letras A-F o números 0-9) en MAYÚSCULAS.' });
    }

    try {
        const response = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!A:A`, // Solo leer la columna de IDs
        });
        const rows = response.data.values;

        if (rows && rows.some(row => row[0] === id)) {
            return res.status(409).json({ error: 'El ID de alumno ya existe.' });
        }

        // Si usas bcrypt:
        // const hashedPassword = await bcrypt.hash(password, 10);
        // const newUserRow = [id, name, sexo, 0, 0, 0, '{}', '', hashedPassword, 0, 0, '']; // ID, Nombre, Sexo, Monedas, Tareas, Asistencias, Compras, Código, Contraseña, EXP, Last_Login_Level, Recovery_Code
        
        // Para tu formato actual (contraseña en claro)
        const newUserRow = [id, name, sexo, 0, 0, 0, '{}', '', password, 0, 0, '']; // ID, Nombre, Sexo, Monedas, Tareas, Asistencias, Compras, Código, Contraseña, EXP, Last_Login_Level, Recovery_Code

        await sheets.spreadsheets.values.append({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!A:L`, // Rango de columnas a las que se añadirán datos
            valueInputOption: 'RAW',
            insertDataOption: 'INSERT_ROWS',
            requestBody: {
                values: [newUserRow],
            },
        });

        res.status(201).json({ success: true, message: 'Usuario registrado exitosamente.' });

    } catch (error) {
        console.error('Error al registrar usuario:', error);
        res.status(500).json({ error: 'Error interno del servidor.' });
    }
});

// Endpoint para enviar código de reseteo de contraseña
app.post('/api/send-reset-code', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { id, career } = req.body;

    if (!id || !career) {
        return res.status(400).json({ error: 'ID de alumno y carrera son requeridos.' });
    }

    const sheetName = getSheetName(career);

    try {
        const response = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!A:L`, // Columna A (ID) y K (Recovery_Code)
        });
        const rows = response.data.values;

        if (!rows) {
            return res.status(404).json({ error: 'No se encontraron alumnos en esta carrera.' });
        }

        let studentRowIndex = -1;
        let studentEmail = null; // Asumiendo que hay una columna para el email si lo necesitas

        for (let i = 1; i < rows.length; i++) {
            if (rows[i][0] === id) { // Columna A es el ID
                studentRowIndex = i + 1; // Número de fila en Sheets (base 1)
                studentEmail = rows[i][7]; // Columna H (si tienes email allí)
                break;
            }
        }

        if (studentRowIndex === -1) {
            return res.status(404).json({ error: 'ID de alumno no encontrado en la carrera seleccionada.' });
        }

        // Generar un código de recuperación (ej. 6 dígitos)
        const recoveryCode = Math.floor(100000 + Math.random() * 900000).toString();

        // Guardar el código en la hoja de cálculo (Columna L)
        await sheets.spreadsheets.values.update({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!L${studentRowIndex}`, // Columna L (Recovery_Code)
            valueInputOption: 'RAW',
            requestBody: {
                values: [[recoveryCode]],
            },
        });

        // Enviar el código por correo electrónico (o mostrarlo en la consola para desarrollo)
        // Necesitas configurar Nodemailer con tus credenciales de correo
        const transporter = nodemailer.createTransport({
            service: 'gmail', // O tu servicio de correo
            auth: {
                user: 'tu_correo@gmail.com', // Tu correo
                pass: 'tu_contraseña_app_o_pass' // Contraseña de aplicación si usas Gmail
            }
        });

        const mailOptions = {
            from: 'tu_correo@gmail.com',
            to: 'correo_destino_del_alumno@ejemplo.com', // Esto debe ser dinámico (desde la hoja de cálculo) o simplemente un dummy
            subject: 'Código de Recuperación de Contraseña',
            text: `Tu código de recuperación es: ${recoveryCode}. Este código expirará en 10 minutos.`
        };

        // En producción, probablemente no querrías enviar correos a IDs aleatorios.
        // Asegúrate de que `studentEmail` contenga el correo real del alumno.
        // Por ahora, solo lo imprimiremos en consola:
        console.log(`Código de recuperación para ${id}: ${recoveryCode}`);
        console.log(`Enviado (simulado) a: ${studentEmail || 'correo_del_alumno@ejemplo.com'}`);


        // Comentar la siguiente línea si no tienes un servicio de correo configurado
        /*
        await transporter.sendMail(mailOptions);
        */

        res.json({ success: true, message: 'Código de recuperación enviado. Revisa tu correo o consúltalo en consola (solo desarrollo).' });

    } catch (error) {
        console.error('Error al enviar código de recuperación:', error);
        res.status(500).json({ error: 'Error interno del servidor al enviar código.' });
    }
});

// Endpoint para resetear contraseña con código
app.post('/api/reset-password-with-code', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { id, code, password, career } = req.body;

    if (!id || !code || !password || !career) {
        return res.status(400).json({ error: 'ID de alumno, código y nueva contraseña son requeridos.' });
    }

    const sheetName = getSheetName(career);

    // Validar formato de nueva contraseña (4 caracteres HEX mayúsculas)
    const passwordRegex = /^[A-F0-9]{4}$/;
    if (!passwordRegex.test(password)) {
      return res.status(400).json({ error: 'La nueva contraseña debe ser de 4 caracteres (letras A-F o números 0-9) en MAYÚSCULAS.' });
    }

    try {
        const response = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!A:L`, // Columna A (ID), I (Contraseña), L (Recovery_Code)
        });
        const rows = response.data.values;

        if (!rows) {
            return res.status(404).json({ error: 'No se encontraron alumnos en esta carrera.' });
        }

        let studentRowIndex = -1;
        let storedCode = null;

        for (let i = 1; i < rows.length; i++) {
            if (rows[i][0] === id) { // Columna A es el ID
                studentRowIndex = i + 1; // Número de fila en Sheets (base 1)
                storedCode = rows[i][11]; // Columna L es el Recovery_Code
                break;
            }
        }

        if (studentRowIndex === -1) {
            return res.status(404).json({ error: 'ID de alumno no encontrado en la carrera seleccionada.' });
        }

        if (code !== storedCode) {
            return res.status(401).json({ error: 'Código de recuperación incorrecto.' });
        }

        // Si usas bcrypt:
        // const hashedPassword = await bcrypt.hash(password, 10);
        // const updates = [
        //     [`${sheetName}!I${studentRowIndex}`, hashedPassword], // Columna I: Contraseña
        //     [`${sheetName}!L${studentRowIndex}`, ''] // Borrar el código de recuperación
        // ];

        // Para tu formato actual (contraseña en claro)
        const updates = [
            [`${sheetName}!I${studentRowIndex}`, password], // Columna I: Contraseña
            [`${sheetName}!L${studentRowIndex}`, ''] // Borrar el código de recuperación
        ];

        // Ejecutar las actualizaciones
        await sheets.spreadsheets.values.batchUpdate({
            spreadsheetId: SPREADSHEET_ID,
            requestBody: {
                data: updates.map(([range, value]) => ({
                    range: range,
                    values: [[value]]
                })),
                valueInputOption: 'RAW'
            }
        });

        res.json({ success: true, message: 'Contraseña actualizada exitosamente.' });

    } catch (error) {
        console.error('Error al resetear contraseña con código:', error);
        res.status(500).json({ error: 'Error interno del servidor al resetear contraseña.' });
    }
});


// Endpoint para obtener insignias
app.get('/api/badges', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { career } = req.query; // Recibe la carrera como query parameter

    if (!career) {
        return res.status(400).json({ error: 'Carrera requerida para obtener insignias.' });
    }

    const sheetName = getSheetName(career);

    try {
        const response = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!M:N`, // Columna M: Nombre de insignia, Columna N: Cantidad disponible
        });
        const rows = response.data.values;

        if (!rows || rows.length < 2) { // Asume que la primera fila es el encabezado
            return res.status(200).json({ badges: [] });
        }

        // Asumiendo que la columna M tiene el nombre de la insignia y la N tiene la cantidad y el costo
        // Ejemplo de formato en celda N: "10 (50)" donde 10 es cantidad y 50 es costo
        const badges = rows.slice(1).map(row => {
            const name = row[0]; // Columna M
            const quantityAndCostString = row[1] || '0 (0)'; // Columna N
            const match = quantityAndCostString.match(/(\d+)\s*\((\d+)\)/);
            let quantity = 0;
            let cost = 0;
            if (match) {
                quantity = parseInt(match[1]);
                cost = parseInt(match[2]);
            }
            return { name, quantity, cost };
        });

        res.json({ success: true, badges });
    } catch (error) {
        console.error('Error al obtener insignias:', error);
        res.status(500).json({ error: 'Error al obtener insignias', details: error.message });
    }
});

// Endpoint para la compra de insignias
app.post('/api/purchase', async (req, res) => {
    if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
    const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost, career } = req.body;

    if (!studentId || studentRowIndex === undefined || !itemName || quantityToBuy === undefined || itemCost === undefined || !career) {
        return res.status(400).json({ error: 'Datos de compra incompletos.' });
    }

    const sheetName = getSheetName(career);

    try {
        // 1. Obtener datos actuales del alumno y de la insignia
        const studentResponse = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!D${studentRowIndex + 1}:G${studentRowIndex + 1}`, // Monedas (D) y Compras (G)
        });
        const studentData = studentResponse.data.values[0];
        let currentCoins = parseInt(studentData[0]) || 0; // Columna D (índice 0 en el rango)
        let currentPurchasesString = studentData[3] || '{}'; // Columna G (índice 3 en el rango)
        let currentPurchases = {};
        try {
            currentPurchases = JSON.parse(currentPurchasesString);
        } catch (e) {
            console.error('Error al parsear compras actuales:', e);
        }

        const badgesResponse = await sheets.spreadsheets.values.get({
            spreadsheetId: SPREADSHEET_ID,
            range: `${sheetName}!M:N`, // Columna M: Nombre de insignia, Columna N: Cantidad disponible
        });
        const badgeRows = badgesResponse.data.values;
        const badgeHeader = badgeRows[0];
        const badgeItemIndex = badgeRows.findIndex(row => row[0] === itemName);
        
        if (badgeItemIndex === -1) {
            return res.status(404).json({ error: 'Insignia no encontrada.' });
        }
        
        const badgeRow = badgeRows[badgeItemIndex];
        const currentQuantityAndCostString = badgeRow[1] || '0 (0)';
        const match = currentQuantityAndCostString.match(/(\d+)\s*\((\d+)\)/);
        let currentBadgeQuantity = 0;
        // let actualItemCost = 0; // Podrías validar que itemCost coincida con este

        if (match) {
            currentBadgeQuantity = parseInt(match[1]);
            // actualItemCost = parseInt(match[2]);
        }

        // 2. Validaciones
        const totalCost = quantityToBuy * itemCost;
        if (currentCoins < totalCost) {
            return res.status(400).json({ error: 'Monedas insuficientes.' });
        }
        if (currentBadgeQuantity < quantityToBuy) {
            return res.status(400).json({ error: 'No hay suficientes insignias disponibles.' });
        }

        // 3. Actualizar datos
        const newCoins = currentCoins - totalCost;
        const newBadgeQuantity = currentBadgeQuantity - quantityToBuy;
        currentPurchases[itemName] = (currentPurchases[itemName] || 0) + quantityToBuy;
        const newPurchasesString = JSON.stringify(currentPurchases);

        // Actualizar hoja de cálculo
        const updates = [
            {
                range: `${sheetName}!D${studentRowIndex + 1}`, // Columna D: Monedas
                values: [[newCoins]],
            },
            {
                range: `${sheetName}!G${studentRowIndex + 1}`, // Columna G: Compras
                values: [[newPurchasesString]],
            },
            {
                range: `${sheetName}!N${badgeItemIndex + 1}`, // Columna N: Cantidad de insignia disponible
                values: [[`${newBadgeQuantity} (${itemCost})`]], // Mantener el formato "cantidad (costo)"
            },
        ];

        await sheets.spreadsheets.values.batchUpdate({
            spreadsheetId: SPREADSHEET_ID,
            requestBody: {
                data: updates,
                valueInputOption: 'RAW',
            },
        });

        res.json({ success: true, message: 'Compra exitosa.', newCoins, newBadgeQuantity });

    } catch (error) {
        console.error('Error al procesar la compra:', error);
        res.status(500).json({ error: 'Error interno del servidor al procesar la compra.' });
    }
});

// Endpoint para añadir EXP.
app.post('/api/add-exp', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { studentId, expToAdd, career } = req.body;

  if (!studentId || expToAdd === undefined || expToAdd < 0 || !career) {
    return res.status(400).json({ error: 'ID de alumno, EXP. a añadir y carrera son requeridos y la EXP. debe ser positiva.' });
  }

  const sheetName = getSheetName(career);

  try {
    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: `${sheetName}!A:K`, // Leer hasta la columna K para obtener ID, EXP. y LAST_LOGIN_LEVEL
    });
    const rows = response.data.values;

    if (!rows || rows.length < 2) {
      return res.status(404).json({ error: 'No se encontraron alumnos en esta carrera.' });
    }

    let studentRow = null;
    let index = -1;

    for (let i = 1; i < rows.length; i++) {
      if (rows[i][0] === studentId) { // Columna A es el ID
        studentRow = rows[i];
        index = i; // Índice de la fila en el array (no en Sheets)
        break;
      }
    }

    if (!studentRow) {
      return res.status(404).json({ error: 'Alumno no encontrado en la carrera seleccionada.' });
    }

    const rowNumber = index + 1; // Número de fila en Sheets (base 1)
    const currentExp = parseInt(studentRow[9]) || 0; // Columna J (índice 9) es EXP.
    let currentLastLoginLevel = parseInt(studentRow[10]); // Columna K (índice 10) es LAST_LOGIN_LEVEL

    if (isNaN(currentLastLoginLevel)) { // Si está vacío, inicialízalo a 0
        currentLastLoginLevel = 0;
    }

    const newExp = currentExp + expToAdd;
    const newLevel = calculateLevel(newExp);

    const updates = [{
      range: `${sheetName}!J${rowNumber}`, // Columna J: EXP. en la hoja correcta
      values: [[newExp]]
    }];

    // Actualizar LAST_LOGIN_LEVEL solo si el nuevo nivel es mayor o si estaba vacío
    if (newLevel > currentLastLoginLevel || isNaN(parseInt(studentRow[10]))) {
        updates.push({
            range: `${sheetName}!K${rowNumber}`, // Columna K: LAST_LOGIN_LEVEL en la hoja correcta
            values: [[newLevel]]
        });
    }

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: updates,
        valueInputOption: 'RAW'
      }
    });

    res.json({
      success: true,
      message: `EXP. de ${studentId} actualizada. Nueva EXP.: ${newExp}, Nuevo Nivel: ${newLevel}.`
    });

  } catch (error) {
    console.error('Error al añadir EXP.:', error);
    res.status(500).json({ error: 'Error interno del servidor al añadir EXP.', details: error.message });
  }
});

// Endpoint para el Leaderboard
app.get('/api/leaderboard', async (req, res) => {
  if (!sheets) return res.status(500).json({ error: 'Sheets no inicializado' });
  const { career } = req.query; // Recibe la carrera como query parameter

  if (!career) {
    return res.status(400).json({ error: 'Carrera requerida para el leaderboard.' });
  }

  const sheetName = getSheetName(career);

  try {
    const range = `${sheetName}!A:J`; // Columnas A (ID), B (NOMBRE), J (EXP)
    const response = await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range });
    const rows = response.data.values;

    if (!rows || rows.length < 2) { // Asume que la primera fila es el encabezado
      return res.status(200).json({ leaderboard: [] }); // Retorna array vacío si no hay datos
    }

    // Mapear los datos a un formato más manejable y calcular el nivel
    const leaderboardData = rows.slice(1).map(row => ({
      id: row[0], // Columna A
      name: row[1] || 'N/A', // Columna B (necesario para el avatar por defecto)
      sexo: row[2] || 'N/A', // Columna C (necesario para el avatar por defecto)
      exp: parseInt(row[9]) || 0, // Columna J es EXP
      level: calculateLevel(parseInt(row[9]) || 0) // Calcular el nivel para el avatar
    }));

    // Ordenar por EXP (puntos) de mayor a menor
    leaderboardData.sort((a, b) => b.exp - a.exp);

    res.json({ success: true, leaderboard: leaderboardData });
  } catch (error) {
    console.error('Error al obtener el leaderboard:', error);
    res.status(500).json({ error: 'Error al obtener el leaderboard', details: error.message });
  }
});

// ===============================================
// === INICIAR EL SERVIDOR ===
// ===============================================
app.listen(PORT, () => {
  console.log(`Servidor Express corriendo en el puerto ${PORT}`);
});