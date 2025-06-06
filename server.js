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
    console.log('Cliente de Google Sheets inicializado correctamente.');
  } catch (error) {
    console.error('Error al inicializar el cliente de Google Sheets:', error);
    process.exit(1); // Salir si no se puede inicializar Google Sheets
  }
}

// Iniciar la inicialización del cliente de Google Sheets
initializeGoogleSheetsClient();

// Función auxiliar para calcular el nivel del alumno
const calculateLevel = (exp) => {
  if (exp >= 100) return 5;
  if (exp >= 75) return 4;
  if (exp >= 50) return 3;
  if (exp >= 25) return 2;
  if (exp >= 10) return 1;
  return 0;
};

// Ruta de inicio de sesión
app.post('/api/login', async (req, res) => {
  const { studentId, password, career } = req.body;
  if (!studentId || !password || !career) {
    return res.status(400).json({ error: 'Faltan ID, contraseña o carrera.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    const range = 'Sheet1!A:K'; // Rango que cubre todas las columnas relevantes

    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: range,
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron datos de alumnos.' });
    }

    const header = rows[0];
    const idIndex = header.indexOf('ID');
    const passIndex = header.indexOf('CONTRASEÑA');
    const nameIndex = header.indexOf('NOMBRE');
    const sexoIndex = header.indexOf('SEXO');
    const monedasIndex = header.indexOf('MONEDAS');
    const expIndex = header.indexOf('EXP.');
    const tareasIndex = header.indexOf('TAREAS');
    const asistenciasIndex = header.indexOf('ASISTENCIAS');
    const purchasesIndex = header.indexOf('COMPRAS');
    const careerIndex = header.indexOf('CARRERA');
    const lastLoginLevelIndex = header.indexOf('LAST_LOGIN_LEVEL');

    if (idIndex === -1 || passIndex === -1 || nameIndex === -1 || sexoIndex === -1 || monedasIndex === -1 || expIndex === -1 || tareasIndex === -1 || asistenciasIndex === -1 || purchasesIndex === -1 || careerIndex === -1 || lastLoginLevelIndex === -1) {
      return res.status(500).json({ error: 'Faltan columnas esenciales en la hoja de cálculo.' });
    }

    let studentFound = false;
    let studentData = null;
    let rowIndex = -1; // Para guardar el índice de la fila real del alumno

    for (let i = 1; i < rows.length; i++) { // Empezar desde 1 para saltar el encabezado
      const row = rows[i];
      if (row[idIndex] === studentId && row[passIndex] === password && row[careerIndex] === career) {
        studentFound = true;
        rowIndex = i + 1; // La fila en Google Sheets es 1-indexed
        studentData = {
          id: row[idIndex],
          name: row[nameIndex],
          sexo: row[sexoIndex],
          monedas: row[monedasIndex] || '0',
          exp: parseInt(row[expIndex]) || 0,
          tareas: parseInt(row[tareasIndex]) || 0,
          asistencias: parseInt(row[asistenciasIndex]) || 0,
          career: row[careerIndex],
          // Parsear las compras desde el formato de cadena a un objeto JS
          purchases: JSON.parse(row[purchasesIndex] || '{}'),
          rowIndex: rowIndex, // Guardar el índice de la fila para futuras actualizaciones
        };

        // Calcular el nivel actual del alumno
        const currentLevel = calculateLevel(studentData.exp);
        studentData.level = currentLevel;

        // Recuperar el nivel de la última vez que el usuario inició sesión
        const lastLoginLevel = parseInt(row[lastLoginLevelIndex]) || 0;

        // Verificar si el nivel ha subido y actualizar LAST_LOGIN_LEVEL si es necesario
        let levelUpOccurred = false;
        if (currentLevel > lastLoginLevel) {
          levelUpOccurred = true;
          // Actualizar LAST_LOGIN_LEVEL en la hoja de cálculo
          await sheets.spreadsheets.values.update({
            spreadsheetId: SPREADSHEET_ID,
            range: `Sheet1!K${rowIndex}`, // Columna K es LAST_LOGIN_LEVEL
            valueInputOption: 'RAW',
            requestBody: {
              values: [[currentLevel]],
            },
          });
        }
        
        return res.json({ success: true, student: studentData, levelUpOccurred });
      }
    }

    if (!studentFound) {
      return res.status(401).json({ success: false, error: 'ID, contraseña o carrera incorrectos.' });
    }

  } catch (error) {
    console.error('Error en la API de inicio de sesión:', error);
    res.status(500).json({ error: 'Error interno del servidor.' });
  }
});

// Ruta para el registro de nuevos usuarios
app.post('/api/register', async (req, res) => {
  const { id, name, sexo, password, career } = req.body;
  if (!id || !name || !sexo || !password || !career) {
    return res.status(400).json({ error: 'Faltan datos de registro.' });
  }

  // Validación de formato de contraseña (4 caracteres, solo mayúsculas y números)
  const passwordRegex = /^[A-Z0-9]{4}$/;
  if (!passwordRegex.test(password)) {
    return res.status(400).json({ error: 'La contraseña debe ser de 4 letras/números MAYÚSCULOS.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    // Verificar si el ID ya existe en la carrera seleccionada
    const checkResponse = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A:K', // Leer hasta la columna K
    });
    const rows = checkResponse.data.values || [];

    const header = rows[0];
    const idIndex = header.indexOf('ID');
    const careerIndex = header.indexOf('CARRERA');

    if (idIndex === -1 || careerIndex === -1) {
        return res.status(500).json({ error: 'Columnas ID o CARRERA no encontradas en la hoja de cálculo.' });
    }

    for (let i = 1; i < rows.length; i++) {
        if (rows[i][idIndex] === id && rows[i][careerIndex] === career) {
            return res.status(409).json({ error: `El ID ${id} ya está registrado para la carrera de ${career}.` });
        }
    }

    // Si el ID no existe para esa carrera, añadir el nuevo alumno
    const newRow = [
      id,
      name,
      sexo,
      password,
      '0', // MONEDAS (valor inicial)
      '0', // EXP. (valor inicial)
      '0', // TAREAS (valor inicial)
      '0', // ASISTENCIAS (valor inicial)
      JSON.stringify({}), // COMPRAS (objeto JSON vacío)
      career, // CARRERA
      '0' // LAST_LOGIN_LEVEL (valor inicial)
    ];

    await sheets.spreadsheets.values.append({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A1', // Añadir al final de Sheet1
      valueInputOption: 'RAW',
      requestBody: {
        values: [newRow],
      },
    });

    res.status(201).json({ success: true, message: 'Usuario registrado exitosamente.' });

  } catch (error) {
    console.error('Error en la API de registro:', error);
    res.status(500).json({ error: 'Error interno del servidor al registrar usuario.' });
  }
});

// Ruta para solicitar código de recuperación de contraseña
const recoveryCodes = {}; // Almacenar códigos de recuperación temporalmente
const transporter = nodemailer.createTransport({
  service: 'gmail', // Puedes usar otro servicio o SMTP
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASS,
  },
});

app.post('/api/send-reset-code', async (req, res) => {
  const { id, career } = req.body;
  if (!id || !career) {
    return res.status(400).json({ error: 'Faltan ID de alumno o carrera.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A:K', // Rango completo para encontrar el alumno
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron datos de alumnos.' });
    }

    const header = rows[0];
    const idIndex = header.indexOf('ID');
    const emailIndex = header.indexOf('EMAIL'); // Asume que tienes una columna EMAIL
    const nameIndex = header.indexOf('NOMBRE');
    const careerIndex = header.indexOf('CARRERA');

    if (idIndex === -1 || emailIndex === -1 || nameIndex === -1 || careerIndex === -1) {
      return res.status(500).json({ error: 'Columnas necesarias (ID, EMAIL, NOMBRE, CARRERA) no encontradas.' });
    }

    let studentEmail = null;
    let studentName = null;
    let studentRowIndex = -1;

    for (let i = 1; i < rows.length; i++) {
      if (rows[i][idIndex] === id && rows[i][careerIndex] === career) {
        studentEmail = rows[i][emailIndex];
        studentName = rows[i][nameIndex];
        studentRowIndex = i + 1; // 1-indexed row number
        break;
      }
    }

    if (!studentEmail) {
      return res.status(404).json({ error: 'Alumno no encontrado para la carrera o sin email registrado.' });
    }

    // Generar un código de 6 dígitos
    const code = Math.floor(100000 + Math.random() * 900000).toString();
    recoveryCodes[id] = { code, timestamp: Date.now(), row: studentRowIndex, career }; // Almacenar el código, tiempo y detalles

    // Enviar correo electrónico con el código
    const mailOptions = {
      from: process.env.EMAIL_USER,
      to: studentEmail,
      subject: 'Código de recuperación de contraseña',
      html: `<p>Hola ${studentName},</p>
             <p>Tu código de recuperación de contraseña es: <strong>${code}</strong></p>
             <p>Este código es válido por 10 minutos.</p>
             <p>Si no solicitaste esto, ignora este correo.</p>`,
    };

    transporter.sendMail(mailOptions, (error, info) => {
      if (error) {
        console.error('Error al enviar correo:', error);
        return res.status(500).json({ error: 'Error al enviar el código de recuperación.' });
      }
      console.log('Correo enviado:', info.response);
      res.json({ message: 'Código de recuperación enviado a tu correo.' });
    });

  } catch (error) {
    console.error('Error en la API de enviar código de recuperación:', error);
    res.status(500).json({ error: 'Error interno del servidor.' });
  }
});

// Ruta para restablecer la contraseña con el código
app.post('/api/reset-password-with-code', async (req, res) => {
  const { id, code, password, career } = req.body;
  if (!id || !code || !password || !career) {
    return res.status(400).json({ error: 'Faltan datos para restablecer la contraseña.' });
  }

  // Validación de formato de nueva contraseña (4 caracteres, solo mayúsculas y números)
  const passwordRegex = /^[A-Z0-9]{4}$/;
  if (!passwordRegex.test(password)) {
    return res.status(400).json({ error: 'La nueva contraseña debe ser de 4 letras/números MAYÚSCULOS.' });
  }

  const storedCodeData = recoveryCodes[id];

  if (!storedCodeData || storedCodeData.code !== code || storedCodeData.career !== career || (Date.now() - storedCodeData.timestamp > 10 * 60 * 1000)) {
    // Código incorrecto, expirado o carrera no coincide
    return res.status(400).json({ error: 'Código inválido, expirado o ID/carrera incorrecto para este código.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    // Actualizar la contraseña en Google Sheets
    // Asumiendo que la contraseña está en la columna D (índice 3)
    await sheets.spreadsheets.values.update({
      spreadsheetId: SPREADSHEET_ID,
      range: `Sheet1!D${storedCodeData.row}`, // Columna D es CONTRASEÑA
      valueInputOption: 'RAW',
      requestBody: {
        values: [[password]],
      },
    });

    delete recoveryCodes[id]; // Eliminar el código después de usarlo
    res.json({ success: true, message: 'Contraseña actualizada exitosamente.' });

  } catch (error) {
    console.error('Error en la API de restablecer contraseña:', error);
    res.status(500).json({ error: 'Error interno del servidor.' });
  }
});


// Ruta para obtener las insignias (badges) disponibles
app.get('/api/badges', async (req, res) => {
  const { career } = req.query; // Obtener la carrera del query parameter
  if (!career) {
      return res.status(400).json({ error: 'Falta el parámetro de carrera para las insignias.' });
  }
  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Badges!A:C', // Asume que las insignias están en una hoja llamada 'Badges'
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.json({ badges: [] });
    }

    const header = rows[0];
    const nameIndex = header.indexOf('NOMBRE');
    const costIndex = header.indexOf('COSTO');
    const quantityIndex = header.indexOf(career); // El nombre de la columna es la carrera

    if (nameIndex === -1 || costIndex === -1 || quantityIndex === -1) {
      return res.status(500).json({ error: 'Columnas necesarias (NOMBRE, COSTO, [CARRERA]) no encontradas en la hoja de insignias.' });
    }

    const badges = rows.slice(1).map(row => ({
      name: row[nameIndex],
      cost: parseInt(row[costIndex]) || 0,
      quantity: parseInt(row[quantityIndex]) || 0, // Cantidad disponible para esa carrera
    }));

    res.json({ badges });

  } catch (error) {
    console.error('Error al obtener insignias:', error);
    res.status(500).json({ error: 'Error interno del servidor al obtener insignias.' });
  }
});

// Ruta para procesar la compra de insignias
app.post('/api/purchase', async (req, res) => {
  const { studentId, studentRowIndex, itemName, quantityToBuy, itemCost, career } = req.body;
  if (!studentId || !studentRowIndex || !itemName || quantityToBuy === undefined || itemCost === undefined || !career) {
    return res.status(400).json({ error: 'Faltan datos para la compra.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    // 1. Obtener datos actuales del alumno y de la insignia
    const studentRange = `Sheet1!A${studentRowIndex}:K${studentRowIndex}`;
    const badgeRange = `Badges!A:C`; // Rango para buscar la insignia

    const [studentResponse, badgesResponse] = await Promise.all([
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: studentRange }),
      sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: badgeRange })
    ]);

    const studentRow = studentResponse.data.values ? studentResponse.data.values[0] : null;
    const allBadgeRows = badgesResponse.data.values || [];

    if (!studentRow) {
      return res.status(404).json({ error: 'Alumno no encontrado.' });
    }
    if (allBadgeRows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron insignias.' });
    }

    // Indices de columnas para el alumno
    const headerAlumno = (await sheets.spreadsheets.values.get({ spreadsheetId: SPREADSHEET_ID, range: 'Sheet1!A1:K1' })).data.values[0];
    const monedasIndexAlumno = headerAlumno.indexOf('MONEDAS');
    const purchasesIndexAlumno = headerAlumno.indexOf('COMPRAS');

    // Indices de columnas para las insignias
    const headerBadges = allBadgeRows[0];
    const nameIndexBadge = headerBadges.indexOf('NOMBRE');
    const quantityIndexBadge = headerBadges.indexOf(career); // La columna de cantidad para la carrera

    if (monedasIndexAlumno === -1 || purchasesIndexAlumno === -1 || nameIndexBadge === -1 || quantityIndexBadge === -1) {
        return res.status(500).json({ error: 'Columnas necesarias no encontradas en las hojas de cálculo.' });
    }


    let currentMonedas = parseInt(studentRow[monedasIndexAlumno]) || 0;
    let currentPurchases = JSON.parse(studentRow[purchasesIndexAlumno] || '{}');

    let badgeFound = false;
    let badgeCurrentQuantity = 0;
    let badgeRowIndex = -1;

    for (let i = 1; i < allBadgeRows.length; i++) {
        if (allBadgeRows[i][nameIndexBadge] === itemName) {
            badgeCurrentQuantity = parseInt(allBadgeRows[i][quantityIndexBadge]) || 0;
            badgeRowIndex = i + 1; // 1-indexed row number for the badge
            badgeFound = true;
            break;
        }
    }

    if (!badgeFound) {
      return res.status(404).json({ error: 'Insignia no encontrada.' });
    }

    // 2. Validaciones
    const totalCost = itemCost * quantityToBuy;
    if (currentMonedas < totalCost) {
      return res.status(400).json({ error: 'Monedas insuficientes.' });
    }
    if (badgeCurrentQuantity < quantityToBuy) {
      return res.status(400).json({ error: `Cantidad insuficiente de ${itemName} disponible.` });
    }

    // 3. Actualizar datos
    const newMonedas = currentMonedas - totalCost;
    const newBadgeQuantity = badgeCurrentQuantity - quantityToBuy;

    currentPurchases[itemName] = (currentPurchases[itemName] || 0) + quantityToBuy;
    const newPurchasesString = JSON.stringify(currentPurchases);

    // Preparar actualizaciones por lotes
    const updates = [
      {
        range: `Sheet1!G${studentRowIndex}`, // Columna G es MONEDAS
        values: [[newMonedas]],
      },
      {
        range: `Sheet1!I${studentRowIndex}`, // Columna I es COMPRAS
        values: [[newPurchasesString]],
      },
      {
        range: `Badges!${String.fromCharCode(65 + quantityIndexBadge)}${badgeRowIndex}`, // Columna de la carrera en Badges
        values: [[newBadgeQuantity]],
      }
    ];

    await sheets.spreadsheets.values.batchUpdate({
      spreadsheetId: SPREADSHEET_ID,
      requestBody: {
        data: updates,
        valueInputOption: 'RAW',
      },
    });

    res.json({
      success: true,
      message: 'Compra realizada con éxito.',
      newCoins: newMonedas.toString(),
      newBadgeQuantity: newBadgeQuantity,
      updatedPurchases: currentPurchases,
    });

  } catch (error) {
    console.error('Error en la API de compra:', error);
    res.status(500).json({ error: 'Error interno del servidor al procesar la compra.' });
  }
});

// Ruta para añadir EXP. (para el administrador)
app.post('/api/add-exp', async (req, res) => {
  const { studentId, expToAdd, career } = req.body;
  if (!studentId || expToAdd === undefined || !career) {
    return res.status(400).json({ error: 'Faltan ID de alumno, EXP. a añadir o carrera.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: 'Sheet1!A:K', // Leer hasta LAST_LOGIN_LEVEL
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.status(404).json({ error: 'No se encontraron datos de alumnos.' });
    }

    const header = rows[0];
    const idIndex = header.indexOf('ID');
    const expIndex = header.indexOf('EXP.');
    const careerIndex = header.indexOf('CARRERA');
    const lastLoginLevelIndex = header.indexOf('LAST_LOGIN_LEVEL');

    if (idIndex === -1 || expIndex === -1 || careerIndex === -1 || lastLoginLevelIndex === -1) {
      return res.status(500).json({ error: 'Columnas necesarias (ID, EXP., CARRERA, LAST_LOGIN_LEVEL) no encontradas en la hoja de cálculo.' });
    }

    let studentFound = false;
    let index = -1;

    for (let i = 1; i < rows.length; i++) {
      if (rows[i][idIndex] === studentId && rows[i][careerIndex] === career) {
        studentFound = true;
        index = i;
        break;
      }
    }

    if (!studentFound) {
      return res.status(404).json({ error: `Alumno con ID ${studentId} y carrera ${career} no encontrado.` });
    }

    const rowNumber = index + 1; // Fila real en la hoja de cálculo (1-indexed)
    let currentExp = parseInt(rows[index][expIndex]) || 0;
    let currentLastLoginLevel = parseInt(rows[index][lastLoginLevelIndex]) || 0;

    if (isNaN(currentLastLoginLevel)) { // Si está vacío, inicialízalo a 0
        currentLastLoginLevel = 0;
    }

    const newExp = currentExp + expToAdd;
    const newLevel = calculateLevel(newExp);

    const updates = [{
      range: `Sheet1!H${rowNumber}`, // Columna H es EXP.
      values: [[newExp]]
    }];

    // Actualizar LAST_LOGIN_LEVEL solo si el nuevo nivel es mayor
    if (newLevel > currentLastLoginLevel) {
        updates.push({
            range: `Sheet1!K${rowNumber}`, // Columna K es LAST_LOGIN_LEVEL
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
      message: `EXP. de ${studentId} actualizada. Nueva EXP.: ${newExp}, Nuevo Nivel: ${newLevel}.`,
      newExp: newExp,
      newLevel: newLevel
    });

  } catch (error) {
    console.error('Error al añadir EXP.:', error);
    res.status(500).json({ error: 'Error interno del servidor al añadir EXP.' });
  }
});

// Nueva ruta API para el leaderboard
// ... (código existente antes de /api/leaderboard) ...

// Nueva ruta API para el leaderboard
app.get('/api/leaderboard', async (req, res) => {
  const { career } = req.query; // Obtener la carrera de los parámetros de la URL

  if (!career) {
    return res.status(400).json({ error: 'Falta el parámetro de carrera para el leaderboard.' });
  }

  try {
    const authClient = await auth.getClient();
    sheets = google.sheets({ version: 'v4', auth: authClient });

    const range = 'Sheet1!A:K'; // Asumiendo que los datos están en la Sheet1 y hasta la columna K (LAST_LOGIN_LEVEL)

    const response = await sheets.spreadsheets.values.get({
      spreadsheetId: SPREADSHEET_ID,
      range: range,
    });

    const rows = response.data.values;
    if (!rows || rows.length === 0) {
      return res.json({ leaderboard: [] }); // No hay datos
    }

    const header = rows[0];
    const careerIndex = header.indexOf('CARRERA');
    const idIndex = header.indexOf('ID');
    const nameIndex = header.indexOf('NOMBRE');
    const expIndex = header.indexOf('EXP.');
    const sexoIndex = header.indexOf('SEXO'); // <-- AÑADIR ESTO: Obtener el índice de la columna SEXO

    if (careerIndex === -1 || idIndex === -1 || nameIndex === -1 || expIndex === -1 || sexoIndex === -1) { // <-- AÑADIR sexoIndex a la validación
      return res.status(500).json({ error: 'Columnas necesarias (CARRERA, ID, NOMBRE, EXP., SEXO) no encontradas en la hoja de cálculo.' });
    }

    let leaderboardData = [];

    const dataRows = rows.slice(1); // Excluir la fila de encabezado
    dataRows.forEach(row => {
      const studentCareer = row[careerIndex];
      // Solo incluir estudiantes de la carrera solicitada
      if (studentCareer === career) {
        const studentId = row[idIndex];
        const studentName = row[nameIndex];
        const studentExp = parseInt(row[expIndex]) || 0;
        const studentSex = row[sexoIndex]; // <-- AÑADIR ESTO: Obtener el sexo

        leaderboardData.push({
          id: studentId,
          name: studentName,
          exp: studentExp,
          career: studentCareer,
          sexo: studentSex // <-- AÑADIR ESTO: Incluir el sexo en la respuesta
        });
      }
    });

    // Ordenar los estudiantes por EXP. en orden descendente
    leaderboardData.sort((a, b) => b.exp - a.exp);

    res.json({ leaderboard: leaderboardData });

  } catch (error) {
    console.error('Error al obtener el leaderboard:', error);
    res.status(500).json({ error: 'Error interno del servidor al obtener el leaderboard.' });
  }
});


// Iniciar el servidor
app.listen(PORT, () => {
  console.log(`Servidor iniciado en http://localhost:${PORT}`);
});